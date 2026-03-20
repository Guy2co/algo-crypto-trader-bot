package grid

import (
	"context"
	"fmt"

	"github.com/Guy2co/algo-crypto-trader-bot/internal/exchange"
	"go.uber.org/zap"
)

// buildClientOrderID creates a deterministic, unique order ID that survives
// bot restarts and enables idempotent re-placement.
//
// Format: "{symbol}-{runID[0:8]}-L{index:03d}-{BUY|SELL}"
func buildClientOrderID(symbol, runID string, levelIndex int, side exchange.OrderSide) string {
	shortRun := runID
	if len(shortRun) > 8 {
		shortRun = shortRun[:8]
	}
	return fmt.Sprintf("%s-%s-L%03d-%s", symbol, shortRun, levelIndex, side)
}

// initializeGrid places initial orders when the grid starts fresh.
// Levels below currentPrice get BUY orders; levels above get SELL orders.
// The level immediately below currentPrice acts as the first entry point.
func (g *Strategy) initializeGrid(ctx context.Context, ex exchange.Exchange, currentPrice float64) error { //nolint:unparam
	prices := ComputeLevels(g.state.GridBottom, g.state.GridTop, g.state.GridCount)
	qty := ComputeQuantityPerGrid(g.state.Investment, g.state.GridCount, currentPrice)

	g.state.mu.Lock()
	g.state.QuantityPerGrid = qty
	g.state.InitialPrice = currentPrice
	g.state.Levels = make([]GridLevel, len(prices))
	for i, p := range prices {
		g.state.Levels[i] = GridLevel{
			Index:         i,
			Price:         p,
			BuyClientOID:  buildClientOrderID(g.state.Symbol, g.state.RunID, i, exchange.OrderSideBuy),
			SellClientOID: buildClientOrderID(g.state.Symbol, g.state.RunID, i, exchange.OrderSideSell),
		}
	}
	g.state.mu.Unlock()

	// Place orders: BUYs below price, SELLs above price.
	// Skip the very top level for buys and very bottom level for sells.
	for i := range g.state.Levels {
		level := &g.state.Levels[i]
		if level.Price < currentPrice && i < len(g.state.Levels)-1 {
			orderID, err := g.placeBuy(ctx, ex, i, level.Price, qty)
			if err != nil {
				g.logger.Warn("failed to place initial buy", zap.Int("level", i), zap.Error(err))
				continue
			}
			g.state.mu.Lock()
			g.state.Levels[i].BuyOrderID = orderID
			g.state.mu.Unlock()
		} else if level.Price > currentPrice && i > 0 {
			orderID, err := g.placeSell(ctx, ex, i, level.Price, qty)
			if err != nil {
				g.logger.Warn("failed to place initial sell", zap.Int("level", i), zap.Error(err))
				continue
			}
			g.state.mu.Lock()
			g.state.Levels[i].SellOrderID = orderID
			g.state.mu.Unlock()
		}
	}

	return nil
}

// recoverGrid reconciles persisted state with the exchange's live order book.
// Missing orders are re-placed using their deterministic ClientOrderID.
func (g *Strategy) recoverGrid(ctx context.Context, ex exchange.Exchange) error {
	liveOrders, err := ex.GetOpenOrders(ctx, g.state.Symbol)
	if err != nil {
		return fmt.Errorf("get open orders for recovery: %w", err)
	}

	// Build a set of live ClientOrderIDs.
	liveByClientID := make(map[string]exchange.Order, len(liveOrders))
	for _, o := range liveOrders {
		liveByClientID[o.ClientOrderID] = o
	}

	g.state.mu.Lock()
	qty := g.state.QuantityPerGrid
	levels := g.state.Levels
	g.state.mu.Unlock()

	for i := range levels {
		level := &g.state.Levels[i]

		// Check if expected buy order is live.
		if level.BuyOrderID != 0 {
			if live, ok := liveByClientID[level.BuyClientOID]; ok {
				// Update with current live order ID (may have changed on reconnect).
				g.state.mu.Lock()
				g.state.Levels[i].BuyOrderID = live.OrderID
				g.state.mu.Unlock()
			} else {
				// Re-place missing buy order.
				newID, placeErr := g.placeBuy(ctx, ex, i, level.Price, qty)
				if placeErr != nil {
					g.logger.Warn("recovery: failed to re-place buy", zap.Int("level", i), zap.Error(placeErr))
					continue
				}
				g.state.mu.Lock()
				g.state.Levels[i].BuyOrderID = newID
				g.state.mu.Unlock()
			}
		}

		// Check if expected sell order is live.
		if level.SellOrderID != 0 {
			if live, ok := liveByClientID[level.SellClientOID]; ok {
				g.state.mu.Lock()
				g.state.Levels[i].SellOrderID = live.OrderID
				g.state.mu.Unlock()
			} else {
				newID, placeErr := g.placeSell(ctx, ex, i, level.Price, qty)
				if placeErr != nil {
					g.logger.Warn("recovery: failed to re-place sell", zap.Int("level", i), zap.Error(placeErr))
					continue
				}
				g.state.mu.Lock()
				g.state.Levels[i].SellOrderID = newID
				g.state.mu.Unlock()
			}
		}
	}

	return nil
}

// handleFill is the core rebalancing logic called from OnFill.
//
// When a BUY fills at level[i]: place SELL at level[i+1].
// When a SELL fills at level[i]: place BUY at level[i-1].
func (g *Strategy) handleFill(ctx context.Context, ex exchange.Exchange, event exchange.OrderFillEvent) error {
	g.state.mu.Lock()
	levels := g.state.Levels
	qty := g.state.QuantityPerGrid
	g.state.mu.Unlock()

	// Find which level this fill belongs to.
	levelIdx := -1
	isBuy := false

	for i, l := range levels {
		if event.Side == exchange.OrderSideBuy && l.BuyOrderID == event.OrderID {
			levelIdx = i
			isBuy = true
			break
		}
		if event.Side == exchange.OrderSideSell && l.SellOrderID == event.OrderID {
			levelIdx = i
			break
		}
	}

	if levelIdx == -1 {
		g.logger.Warn("fill for unknown order", zap.Int64("order_id", event.OrderID))
		return nil
	}

	if isBuy {
		return g.onBuyFill(ctx, ex, levelIdx, event, qty)
	}
	return g.onSellFill(ctx, ex, levelIdx, event, qty)
}

func (g *Strategy) onBuyFill(ctx context.Context, ex exchange.Exchange, levelIdx int, event exchange.OrderFillEvent, qty float64) error {
	// Clear the filled buy order.
	g.state.mu.Lock()
	g.state.Levels[levelIdx].BuyOrderID = 0
	g.state.mu.Unlock()

	// Place sell one level above, if not at the top.
	sellIdx := levelIdx + 1
	if sellIdx >= len(g.state.Levels) {
		g.logger.Info("buy filled at top level; no sell to place", zap.Int("level", levelIdx))
		return nil
	}

	g.logger.Info("buy filled; placing sell",
		zap.Int("buy_level", levelIdx),
		zap.Int("sell_level", sellIdx),
		zap.Float64("fill_price", event.Price),
	)

	sellPrice := g.state.Levels[sellIdx].Price
	newID, err := g.placeSell(ctx, ex, sellIdx, sellPrice, qty)
	if err != nil {
		return fmt.Errorf("place sell after buy fill at level %d: %w", sellIdx, err)
	}

	g.state.mu.Lock()
	g.state.Levels[sellIdx].SellOrderID = newID
	g.state.Metrics.TotalFeesPaid += event.Commission
	g.state.mu.Unlock()

	return nil
}

func (g *Strategy) onSellFill(ctx context.Context, ex exchange.Exchange, levelIdx int, event exchange.OrderFillEvent, qty float64) error {
	// Clear the filled sell order.
	g.state.mu.Lock()
	g.state.Levels[levelIdx].SellOrderID = 0
	g.state.mu.Unlock()

	// Place buy one level below, if not at the bottom.
	buyIdx := levelIdx - 1
	if buyIdx < 0 {
		g.logger.Info("sell filled at bottom level; no buy to place", zap.Int("level", levelIdx))
		return nil
	}

	buyPrice := g.state.Levels[buyIdx].Price

	// Compute realized profit for this cycle.
	profit := (event.Price-buyPrice)*qty - event.Commission

	g.logger.Info("sell filled; placing buy",
		zap.Int("sell_level", levelIdx),
		zap.Int("buy_level", buyIdx),
		zap.Float64("sell_price", event.Price),
		zap.Float64("profit_usdt", profit),
	)

	newID, err := g.placeBuy(ctx, ex, buyIdx, buyPrice, qty)
	if err != nil {
		return fmt.Errorf("place buy after sell fill at level %d: %w", buyIdx, err)
	}

	g.state.mu.Lock()
	g.state.Levels[buyIdx].BuyOrderID = newID
	g.state.Metrics.TotalCycles++
	g.state.Metrics.TotalProfit += profit
	g.state.Metrics.TotalFeesPaid += event.Commission
	g.state.Metrics.LastCycleTime = event.EventTime
	g.state.mu.Unlock()

	return nil
}

// placeBuy sends a limit BUY order and returns the exchange-assigned order ID.
func (g *Strategy) placeBuy(ctx context.Context, ex exchange.Exchange, levelIdx int, price, qty float64) (int64, error) {
	clientOID := buildClientOrderID(g.state.Symbol, g.state.RunID, levelIdx, exchange.OrderSideBuy)
	order, err := ex.PlaceLimitOrder(ctx, exchange.PlaceOrderRequest{
		Symbol:        g.state.Symbol,
		Side:          exchange.OrderSideBuy,
		Price:         price,
		Quantity:      qty,
		ClientOrderID: clientOID,
	})
	if err != nil {
		return 0, fmt.Errorf("place buy at level %d price %.2f: %w", levelIdx, price, err)
	}
	g.logger.Debug("buy order placed", zap.Int("level", levelIdx), zap.Float64("price", price), zap.Int64("order_id", order.OrderID))
	return order.OrderID, nil
}

// placeSell sends a limit SELL order and returns the exchange-assigned order ID.
func (g *Strategy) placeSell(ctx context.Context, ex exchange.Exchange, levelIdx int, price, qty float64) (int64, error) {
	clientOID := buildClientOrderID(g.state.Symbol, g.state.RunID, levelIdx, exchange.OrderSideSell)
	order, err := ex.PlaceLimitOrder(ctx, exchange.PlaceOrderRequest{
		Symbol:        g.state.Symbol,
		Side:          exchange.OrderSideSell,
		Price:         price,
		Quantity:      qty,
		ClientOrderID: clientOID,
	})
	if err != nil {
		return 0, fmt.Errorf("place sell at level %d price %.2f: %w", levelIdx, price, err)
	}
	g.logger.Debug("sell order placed", zap.Int("level", levelIdx), zap.Float64("price", price), zap.Int64("order_id", order.OrderID))
	return order.OrderID, nil
}

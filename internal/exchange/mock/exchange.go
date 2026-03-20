// Package mock provides an in-memory Exchange implementation for backtesting
// and unit tests. It simulates order fills against OHLCV candle data.
package mock

import (
	"context"
	"fmt"
	"math"
	"sync"
	"sync/atomic"

	"github.com/Guy2co/algo-crypto-trader-bot/internal/exchange"
)

// Exchange is a fully in-memory exchange that implements exchange.Exchange.
type Exchange struct {
	mu           sync.RWMutex
	currentPrice float64
	balances     map[string]exchange.Balance
	openOrders   map[int64]exchange.Order
	nextOrderID  atomic.Int64
	fillChan     chan exchange.OrderFillEvent
	feeRate      float64
}

// New creates a new mock Exchange with the given initial balances and fee rate.
func New(balances map[string]exchange.Balance, feeRate float64) *Exchange {
	m := &Exchange{
		balances:   make(map[string]exchange.Balance, len(balances)),
		openOrders: make(map[int64]exchange.Order),
		fillChan:   make(chan exchange.OrderFillEvent, 256),
		feeRate:    feeRate,
	}
	for k, v := range balances {
		m.balances[k] = v
	}
	return m
}

// SetPrice updates the mock's current market price.
func (m *Exchange) SetPrice(price float64) {
	m.mu.Lock()
	m.currentPrice = price
	m.mu.Unlock()
}

// SimulateFills processes a candle and fills any open orders whose price
// falls within the candle's [Low, High] range.
// BUY orders fill when order.Price >= candle.Low.
// SELL orders fill when order.Price <= candle.High.
func (m *Exchange) SimulateFills(candle exchange.Candle) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.currentPrice = candle.Close

	for id, order := range m.openOrders {
		filled := false
		switch order.Side {
		case exchange.OrderSideBuy:
			filled = order.Price >= candle.Low
		case exchange.OrderSideSell:
			filled = order.Price <= candle.High
		}

		if !filled {
			continue
		}

		fee := order.Quantity * order.Price * m.feeRate
		filledOrder := order
		filledOrder.Status = exchange.OrderStatusFilled
		filledOrder.FilledQty = order.Quantity
		delete(m.openOrders, id)

		// Update balances
		m.applyFill(filledOrder, fee)

		m.fillChan <- exchange.OrderFillEvent{
			OrderID:         filledOrder.OrderID,
			ClientOrderID:   filledOrder.ClientOrderID,
			Symbol:          filledOrder.Symbol,
			Side:            filledOrder.Side,
			Price:           filledOrder.Price,
			Quantity:        filledOrder.Quantity,
			CumulativeQty:   filledOrder.Quantity,
			Status:          exchange.OrderStatusFilled,
			Commission:      fee,
			CommissionAsset: "USDT",
			EventTime:       candle.CloseTime,
			TradeID:         id,
		}
	}
}

// applyFill updates balances when an order fills. Must be called with mu held.
func (m *Exchange) applyFill(order exchange.Order, fee float64) {
	switch order.Side {
	case exchange.OrderSideBuy:
		// Release locked USDT, credit base asset minus fee
		cost := order.Quantity * order.Price
		quote := m.balances["USDT"]
		quote.Locked = math.Max(0, quote.Locked-cost)
		m.balances["USDT"] = quote

		symbol := order.Symbol
		baseAsset := symbol[:len(symbol)-4] // e.g. "BTC" from "BTCUSDT"
		base := m.balances[baseAsset]
		base.Free += order.Quantity - (fee / order.Price)
		m.balances[baseAsset] = base

	case exchange.OrderSideSell:
		// Release locked base asset, credit USDT minus fee
		base := m.balances[order.Symbol[:len(order.Symbol)-4]]
		base.Locked = math.Max(0, base.Locked-order.Quantity)
		m.balances[order.Symbol[:len(order.Symbol)-4]] = base

		quote := m.balances["USDT"]
		quote.Free += order.Quantity*order.Price - fee
		m.balances["USDT"] = quote
	}
}

// --- exchange.Exchange interface implementation ---

func (m *Exchange) GetBalances(_ context.Context) ([]exchange.Balance, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	balances := make([]exchange.Balance, 0, len(m.balances))
	for _, b := range m.balances {
		balances = append(balances, b)
	}
	return balances, nil
}

func (m *Exchange) GetBalance(_ context.Context, asset string) (exchange.Balance, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if b, ok := m.balances[asset]; ok {
		return b, nil
	}
	return exchange.Balance{Asset: asset}, nil
}

func (m *Exchange) PlaceLimitOrder(_ context.Context, req exchange.PlaceOrderRequest) (*exchange.Order, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Validate balance
	switch req.Side {
	case exchange.OrderSideBuy:
		cost := req.Quantity * req.Price
		quote := m.balances["USDT"]
		if quote.Free < cost {
			return nil, fmt.Errorf("insufficient USDT balance: need %.8f, have %.8f", cost, quote.Free)
		}
		quote.Free -= cost
		quote.Locked += cost
		m.balances["USDT"] = quote

	case exchange.OrderSideSell:
		baseAsset := req.Symbol[:len(req.Symbol)-4]
		base := m.balances[baseAsset]
		if base.Free < req.Quantity {
			return nil, fmt.Errorf("insufficient %s balance: need %.8f, have %.8f", baseAsset, req.Quantity, base.Free)
		}
		base.Free -= req.Quantity
		base.Locked += req.Quantity
		m.balances[baseAsset] = base
	}

	orderID := m.nextOrderID.Add(1)
	order := exchange.Order{
		OrderID:       orderID,
		ClientOrderID: req.ClientOrderID,
		Symbol:        req.Symbol,
		Side:          req.Side,
		Price:         req.Price,
		Quantity:      req.Quantity,
		Status:        exchange.OrderStatusNew,
	}
	m.openOrders[orderID] = order
	return &order, nil
}

func (m *Exchange) CancelOrder(_ context.Context, _ string, orderID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	order, ok := m.openOrders[orderID]
	if !ok {
		return fmt.Errorf("order %d not found", orderID)
	}
	m.unlockFunds(order)
	delete(m.openOrders, orderID)
	return nil
}

func (m *Exchange) CancelAllOrders(_ context.Context, symbol string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, order := range m.openOrders {
		if order.Symbol != symbol {
			continue
		}
		m.unlockFunds(order)
		delete(m.openOrders, id)
	}
	return nil
}

// unlockFunds releases the reserved balance for a cancelled order. Must be called with mu held.
func (m *Exchange) unlockFunds(order exchange.Order) {
	switch order.Side {
	case exchange.OrderSideBuy:
		cost := order.Quantity * order.Price
		quote := m.balances["USDT"]
		quote.Locked = math.Max(0, quote.Locked-cost)
		quote.Free += cost
		m.balances["USDT"] = quote
	case exchange.OrderSideSell:
		baseAsset := order.Symbol[:len(order.Symbol)-4]
		base := m.balances[baseAsset]
		base.Locked = math.Max(0, base.Locked-order.Quantity)
		base.Free += order.Quantity
		m.balances[baseAsset] = base
	}
}

func (m *Exchange) GetOrder(_ context.Context, _ string, orderID int64) (*exchange.Order, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if order, ok := m.openOrders[orderID]; ok {
		return &order, nil
	}
	return nil, fmt.Errorf("order %d not found", orderID)
}

func (m *Exchange) GetOpenOrders(_ context.Context, symbol string) ([]exchange.Order, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	orders := make([]exchange.Order, 0)
	for _, o := range m.openOrders {
		if o.Symbol == symbol {
			orders = append(orders, o)
		}
	}
	return orders, nil
}

func (m *Exchange) GetCurrentPrice(_ context.Context, _ string) (float64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentPrice, nil
}

func (m *Exchange) GetCandles(_ context.Context, _ string, _ string, _ int) ([]exchange.Candle, error) {
	return nil, nil
}

func (m *Exchange) SubscribeOrderFills(_ context.Context, _ string) (<-chan exchange.OrderFillEvent, context.CancelFunc, error) {
	return m.fillChan, func() {}, nil
}

func (m *Exchange) FormatQuantity(_ string, qty float64) (string, error) {
	return fmt.Sprintf("%.8f", qty), nil
}

func (m *Exchange) FormatPrice(_ string, price float64) (string, error) {
	return fmt.Sprintf("%.2f", price), nil
}

// GetBookTicker returns a simulated 0.01% spread around the current price.
func (m *Exchange) GetBookTicker(_ context.Context, symbol string) (exchange.BookTicker, error) {
	m.mu.RLock()
	price := m.currentPrice
	m.mu.RUnlock()

	spread := price * 0.0001
	return exchange.BookTicker{
		Symbol:   symbol,
		BidPrice: price - spread,
		BidQty:   100,
		AskPrice: price + spread,
		AskQty:   100,
	}, nil
}

// PlaceMarketOrder fills immediately at the current price, deducting fees.
func (m *Exchange) PlaceMarketOrder(ctx context.Context, req exchange.MarketOrderRequest) (*exchange.Order, error) {
	m.mu.Lock()
	price := m.currentPrice
	m.mu.Unlock()

	limitReq := exchange.PlaceOrderRequest{
		Symbol:        req.Symbol,
		Side:          req.Side,
		Price:         price,
		Quantity:      req.Quantity,
		ClientOrderID: req.ClientOrderID,
	}
	order, err := m.PlaceLimitOrder(ctx, limitReq)
	if err != nil {
		return nil, err
	}

	// Immediately simulate the fill.
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.openOrders[order.OrderID]; !exists {
		// Already removed (shouldn't happen)
		return order, nil
	}

	fee := req.Quantity * price * m.feeRate
	filledOrder := *order
	filledOrder.Status = exchange.OrderStatusFilled
	filledOrder.FilledQty = req.Quantity
	delete(m.openOrders, order.OrderID)

	m.applyFill(filledOrder, fee)

	m.fillChan <- exchange.OrderFillEvent{
		OrderID:         filledOrder.OrderID,
		ClientOrderID:   filledOrder.ClientOrderID,
		Symbol:          filledOrder.Symbol,
		Side:            filledOrder.Side,
		Price:           price,
		Quantity:        req.Quantity,
		CumulativeQty:   req.Quantity,
		Status:          exchange.OrderStatusFilled,
		Commission:      fee,
		CommissionAsset: "USDT",
		TradeID:         order.OrderID,
	}

	filledOrder.Price = price
	return &filledOrder, nil
}

// TotalEquityUSDT returns the total portfolio value in USDT at the current price.
func (m *Exchange) TotalEquityUSDT(baseAsset string) float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	usdtBal := m.balances["USDT"]
	baseBal := m.balances[baseAsset]
	return usdtBal.Total() + baseBal.Total()*m.currentPrice
}

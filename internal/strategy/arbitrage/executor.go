package arbitrage

import (
	"context"
	"fmt"
	"time"

	"github.com/Guy2co/algo-crypto-trader-bot/internal/exchange"
	"go.uber.org/zap"
)

// CycleResult holds the outcome of one arbitrage cycle execution.
type CycleResult struct {
	Path         ArbPath
	StartUSDT    float64
	EndUSDT      float64
	ActualProfit float64
	Duration     time.Duration
	Err          error
	FailedLeg    int // index of first failed leg; -1 if all succeeded
}

// ExecuteCycle executes all legs of an arbitrage path using market orders.
// On partial failure, logs the stuck intermediate position and returns the error.
func ExecuteCycle(
	ctx context.Context,
	path ArbPath,
	startUSDT float64,
	runID string,
	cycleNum int,
	logger *zap.Logger,
) CycleResult {
	start := time.Now()
	result := CycleResult{
		Path:      path,
		StartUSDT: startUSDT,
		FailedLeg: -1,
	}

	// Compute quantity for first leg.
	qty := computeFirstLegQty(path, startUSDT)
	if qty <= 0 {
		result.Err = fmt.Errorf("computed quantity is zero for start=%f", startUSDT)
		return result
	}

	// Execute each leg sequentially.
	for i, leg := range path.Legs {
		clientOID := fmt.Sprintf("arb-%s-c%d-L%d-%s", runID[:8], cycleNum, i, leg.Side)

		_, err := leg.Exchange.PlaceMarketOrder(ctx, exchange.MarketOrderRequest{
			Symbol:        leg.Symbol,
			Side:          leg.Side,
			Quantity:      qty,
			ClientOrderID: clientOID,
		})
		if err != nil {
			result.FailedLeg = i
			result.Err = fmt.Errorf("leg %d (%s %s): %w", i, leg.Side, leg.Symbol, err)
			logger.Error("arbitrage leg failed",
				zap.Int("leg", i),
				zap.String("symbol", leg.Symbol),
				zap.String("side", string(leg.Side)),
				zap.Float64("qty", qty),
				zap.Error(err),
			)
			break
		}

		logger.Debug("arbitrage leg executed",
			zap.Int("leg", i),
			zap.String("symbol", leg.Symbol),
			zap.String("side", string(leg.Side)),
			zap.Float64("qty", qty),
		)

		// Update qty for next leg based on ticker.
		if i+1 < len(path.Legs) {
			qty = nextLegQty(path, i, qty)
		}
	}

	result.Duration = time.Since(start)
	return result
}

// computeFirstLegQty returns the quantity for the first leg given a USDT budget.
//
//	BUY leg:  qty = budget / askPrice (how many base asset we buy)
//	SELL leg: qty = budget / bidPrice (how many base asset we sell)
func computeFirstLegQty(path ArbPath, budgetUSDT float64) float64 {
	if len(path.Legs) == 0 || len(path.Tickers) == 0 {
		return 0
	}
	ticker := path.Tickers[0]
	switch path.Legs[0].Side {
	case exchange.OrderSideBuy:
		if ticker.AskPrice == 0 {
			return 0
		}
		return budgetUSDT / ticker.AskPrice
	case exchange.OrderSideSell:
		if ticker.BidPrice == 0 {
			return 0
		}
		return budgetUSDT / ticker.BidPrice
	}
	return 0
}

// nextLegQty computes the quantity for the (i+1)th leg based on what was
// received from the ith leg.
//
//	Previous BUY:  we received qty_base → next leg uses qty_base
//	Previous SELL: we received qty_base * bidPrice (in quote) → convert
func nextLegQty(path ArbPath, legIdx int, prevQty float64) float64 {
	if legIdx+1 >= len(path.Legs) || legIdx+1 >= len(path.Tickers) {
		return prevQty
	}
	prevLeg := path.Legs[legIdx]
	nextTicker := path.Tickers[legIdx+1]

	switch prevLeg.Side {
	case exchange.OrderSideBuy:
		// Received base asset; next leg trades same base (or uses it directly).
		return prevQty
	case exchange.OrderSideSell:
		// Received quote (USDT-like); convert to next base.
		received := prevQty * path.Tickers[legIdx].BidPrice
		nextLeg := path.Legs[legIdx+1]
		switch nextLeg.Side {
		case exchange.OrderSideBuy:
			if nextTicker.AskPrice == 0 {
				return 0
			}
			return received / nextTicker.AskPrice
		case exchange.OrderSideSell:
			return received
		}
	}
	return prevQty
}

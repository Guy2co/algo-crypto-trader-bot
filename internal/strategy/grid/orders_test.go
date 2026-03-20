package grid

import (
	"context"
	"testing"
	"time"

	"github.com/Guy2co/algo-crypto-trader-bot/internal/exchange"
	mocexchange "github.com/Guy2co/algo-crypto-trader-bot/internal/exchange/mock"
	"github.com/Guy2co/algo-crypto-trader-bot/internal/order"
	"go.uber.org/zap"
)

func newTestStrategy(t *testing.T, bottom, top float64, count int, investment float64) (*Strategy, *mocexchange.Exchange) {
	t.Helper()
	balances := map[string]exchange.Balance{
		"USDT": {Asset: "USDT", Free: investment * 2},
		"BTC":  {Asset: "BTC", Free: 1.0}, // pre-fund BTC so sell orders can be placed
	}
	mock := mocexchange.New(balances, 0.001)
	mid := (bottom + top) / 2
	mock.SetPrice(mid)

	strat := &Strategy{
		state: &GridState{
			RunID:       "test-run-1",
			Symbol:      "BTCUSDT",
			GridBottom:  bottom,
			GridTop:     top,
			GridCount:   count,
			GridSpacing: (top - bottom) / float64(count),
			Investment:  investment,
			Metrics:     GridMetrics{StartTime: time.Now()},
		},
		tracker:  order.NewTracker(),
		stateDir: t.TempDir(),
		logger:   zap.NewNop(),
	}
	return strat, mock
}

func TestInitializeGrid(t *testing.T) {
	t.Parallel()

	strat, mock := newTestStrategy(t, 80000, 100000, 10, 1000)
	ctx := context.Background()

	currentPrice := 90000.0
	mock.SetPrice(currentPrice)

	if err := strat.initializeGrid(ctx, mock, currentPrice); err != nil {
		t.Fatalf("initializeGrid error: %v", err)
	}

	openOrders, err := mock.GetOpenOrders(ctx, "BTCUSDT")
	if err != nil {
		t.Fatalf("GetOpenOrders: %v", err)
	}
	if len(openOrders) == 0 {
		t.Error("expected orders to be placed after initializeGrid")
	}
}

func TestHandleFill_BuySideCreatesUpwardSell(t *testing.T) {
	t.Parallel()

	strat, mock := newTestStrategy(t, 80000, 100000, 10, 1000)
	ctx := context.Background()

	// Build grid state manually.
	levels := ComputeLevels(80000, 100000, 10)
	qty := 0.001
	strat.state.QuantityPerGrid = qty
	strat.state.Levels = make([]GridLevel, len(levels))
	for i, p := range levels {
		strat.state.Levels[i] = GridLevel{
			Index:         i,
			Price:         p,
			BuyClientOID:  buildClientOrderID("BTCUSDT", "test-run-1", i, exchange.OrderSideBuy),
			SellClientOID: buildClientOrderID("BTCUSDT", "test-run-1", i, exchange.OrderSideSell),
		}
	}

	// Place a BUY at level 3 manually.
	order3, err := mock.PlaceLimitOrder(ctx, exchange.PlaceOrderRequest{
		Symbol:   "BTCUSDT",
		Side:     exchange.OrderSideBuy,
		Price:    levels[3],
		Quantity: qty,
	})
	if err != nil {
		t.Fatalf("place manual buy: %v", err)
	}
	strat.state.Levels[3].BuyOrderID = order3.OrderID

	// Simulate fill of that buy.
	fillEvent := exchange.OrderFillEvent{
		OrderID:   order3.OrderID,
		Symbol:    "BTCUSDT",
		Side:      exchange.OrderSideBuy,
		Price:     levels[3],
		Quantity:  qty,
		Status:    exchange.OrderStatusFilled,
		EventTime: time.Now(),
		TradeID:   42,
	}

	if err = strat.handleFill(ctx, mock, fillEvent); err != nil {
		t.Fatalf("handleFill error: %v", err)
	}

	// A SELL should now exist at level 4.
	if strat.state.Levels[4].SellOrderID == 0 {
		t.Error("expected sell order at level 4 after buy fill at level 3")
	}
	// BuyOrderID at level 3 should be cleared.
	if strat.state.Levels[3].BuyOrderID != 0 {
		t.Error("buy order ID should be cleared after fill")
	}
}

func TestHandleFill_SellSideRecordsProfit(t *testing.T) {
	t.Parallel()

	strat, mock := newTestStrategy(t, 80000, 100000, 10, 1000)
	ctx := context.Background()

	levels := ComputeLevels(80000, 100000, 10)
	qty := 0.001
	strat.state.QuantityPerGrid = qty
	strat.state.Levels = make([]GridLevel, len(levels))
	for i, p := range levels {
		strat.state.Levels[i] = GridLevel{
			Index:         i,
			Price:         p,
			BuyClientOID:  buildClientOrderID("BTCUSDT", "test-run-1", i, exchange.OrderSideBuy),
			SellClientOID: buildClientOrderID("BTCUSDT", "test-run-1", i, exchange.OrderSideSell),
		}
	}

	// Place a SELL at level 5.
	sell5, err := mock.PlaceLimitOrder(ctx, exchange.PlaceOrderRequest{
		Symbol:   "BTCUSDT",
		Side:     exchange.OrderSideSell,
		Price:    levels[5],
		Quantity: qty,
	})
	if err != nil {
		t.Fatalf("place sell: %v", err)
	}
	strat.state.Levels[5].SellOrderID = sell5.OrderID

	// Simulate fill.
	fillEvent := exchange.OrderFillEvent{
		OrderID:   sell5.OrderID,
		Symbol:    "BTCUSDT",
		Side:      exchange.OrderSideSell,
		Price:     levels[5],
		Quantity:  qty,
		Status:    exchange.OrderStatusFilled,
		Commission: 0.09,
		EventTime: time.Now(),
		TradeID:   99,
	}

	if err = strat.handleFill(ctx, mock, fillEvent); err != nil {
		t.Fatalf("handleFill error: %v", err)
	}

	// Metrics should be updated.
	if strat.state.Metrics.TotalCycles != 1 {
		t.Errorf("expected 1 cycle, got %d", strat.state.Metrics.TotalCycles)
	}
	if strat.state.Metrics.TotalFeesPaid <= 0 {
		t.Error("expected fees to be recorded")
	}
	// BUY should exist at level 4.
	if strat.state.Levels[4].BuyOrderID == 0 {
		t.Error("expected buy order at level 4 after sell fill at level 5")
	}
}

func TestHandleFill_PartialFillIgnored(t *testing.T) {
	t.Parallel()

	strat, mock := newTestStrategy(t, 80000, 100000, 10, 1000)
	ctx := context.Background()

	strat.state.Levels = []GridLevel{{Index: 0, Price: 80000}}
	mock.SetPrice(80000)

	// A PARTIALLY_FILLED event should be ignored by OnFill (not handleFill directly).
	partialEvent := exchange.OrderFillEvent{
		OrderID: 1,
		Symbol:  "BTCUSDT",
		Side:    exchange.OrderSideBuy,
		Status:  exchange.OrderStatusPartiallyFilled,
		TradeID: 1,
	}

	if err := strat.OnFill(ctx, mock, partialEvent); err != nil {
		t.Fatalf("OnFill error: %v", err)
	}

	// No orders should have been placed.
	orders, _ := mock.GetOpenOrders(ctx, "BTCUSDT")
	if len(orders) != 0 {
		t.Errorf("expected no orders placed for partial fill, got %d", len(orders))
	}
}

func TestHandleFill_DuplicateTradeIDDropped(t *testing.T) {
	t.Parallel()

	strat, mock := newTestStrategy(t, 80000, 100000, 10, 1000)
	ctx := context.Background()
	strat.state.Levels = []GridLevel{{Index: 0, Price: 80000}}

	event := exchange.OrderFillEvent{
		OrderID: 5,
		Symbol:  "BTCUSDT",
		Side:    exchange.OrderSideBuy,
		Status:  exchange.OrderStatusFilled,
		TradeID: 777,
	}

	// First call — processed.
	_ = strat.OnFill(ctx, mock, event)
	cyclesAfterFirst := strat.state.Metrics.TotalCycles

	// Second call — same TradeID, should be a no-op.
	_ = strat.OnFill(ctx, mock, event)
	if strat.state.Metrics.TotalCycles != cyclesAfterFirst {
		t.Error("duplicate fill event should not increment cycle counter")
	}
}

func TestHandleFill_TopLevelBuyNoSellAbove(t *testing.T) {
	t.Parallel()

	strat, mock := newTestStrategy(t, 80000, 100000, 2, 500)
	ctx := context.Background()

	levels := ComputeLevels(80000, 100000, 2)
	strat.state.QuantityPerGrid = 0.001
	strat.state.Levels = make([]GridLevel, len(levels))
	for i, p := range levels {
		strat.state.Levels[i] = GridLevel{Index: i, Price: p}
	}

	// Place buy at the top level (index 2).
	buyOrder, err := mock.PlaceLimitOrder(ctx, exchange.PlaceOrderRequest{
		Symbol: "BTCUSDT", Side: exchange.OrderSideBuy, Price: levels[2], Quantity: 0.001,
	})
	if err != nil {
		t.Fatalf("place buy at top: %v", err)
	}
	strat.state.Levels[2].BuyOrderID = buyOrder.OrderID

	fillEvent := exchange.OrderFillEvent{
		OrderID: buyOrder.OrderID, Symbol: "BTCUSDT",
		Side: exchange.OrderSideBuy, Price: levels[2],
		Quantity: 0.001, Status: exchange.OrderStatusFilled,
		TradeID: 55,
	}

	// Should not panic or return error — just log and skip placing sell.
	if err = strat.handleFill(ctx, mock, fillEvent); err != nil {
		t.Fatalf("handleFill at top level error: %v", err)
	}
}

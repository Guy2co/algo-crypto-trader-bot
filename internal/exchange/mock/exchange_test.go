package mock

import (
	"context"
	"testing"
	"time"

	"github.com/Guy2co/algo-crypto-trader-bot/internal/exchange"
)

func newMock() *Exchange {
	return New(map[string]exchange.Balance{
		"USDT": {Asset: "USDT", Free: 10000},
		"BTC":  {Asset: "BTC", Free: 0},
	}, 0.001)
}

func TestPlaceLimitOrder_Buy(t *testing.T) {
	t.Parallel()

	m := newMock()
	ctx := context.Background()

	order, err := m.PlaceLimitOrder(ctx, exchange.PlaceOrderRequest{
		Symbol:   "BTCUSDT",
		Side:     exchange.OrderSideBuy,
		Price:    90000,
		Quantity: 0.01,
	})
	if err != nil {
		t.Fatalf("PlaceLimitOrder: %v", err)
	}
	if order.OrderID == 0 {
		t.Error("expected non-zero order ID")
	}

	// Verify USDT is locked.
	bal, _ := m.GetBalance(ctx, "USDT")
	wantLocked := 90000.0 * 0.01
	if bal.Locked != wantLocked {
		t.Errorf("locked %.2f, want %.2f", bal.Locked, wantLocked)
	}
}

func TestPlaceLimitOrder_InsufficientBalance(t *testing.T) {
	t.Parallel()

	m := newMock()
	ctx := context.Background()

	_, err := m.PlaceLimitOrder(ctx, exchange.PlaceOrderRequest{
		Symbol:   "BTCUSDT",
		Side:     exchange.OrderSideBuy,
		Price:    90000,
		Quantity: 1, // 90000 USDT needed, only 10000 available
	})
	if err == nil {
		t.Error("expected error for insufficient balance")
	}
}

func TestSimulateFills_BuyFillsWhenPriceTouchesLow(t *testing.T) {
	t.Parallel()

	m := newMock()
	ctx := context.Background()
	m.SetPrice(90000)

	order, _ := m.PlaceLimitOrder(ctx, exchange.PlaceOrderRequest{
		Symbol:   "BTCUSDT",
		Side:     exchange.OrderSideBuy,
		Price:    88000,
		Quantity: 0.01,
	})

	// Candle low touches the buy price.
	candle := exchange.Candle{
		OpenTime:  time.Now(),
		Open:      90000,
		High:      90000,
		Low:       87000, // below 88000 → fill
		Close:     89000,
		CloseTime: time.Now(),
	}
	m.SimulateFills(candle)

	// Order should be removed from open orders.
	orders, _ := m.GetOpenOrders(ctx, "BTCUSDT")
	for _, o := range orders {
		if o.OrderID == order.OrderID {
			t.Error("order should have been removed after fill")
		}
	}

	// Fill event should be in channel.
	fillChan, _, _ := m.SubscribeOrderFills(ctx, "BTCUSDT")
	select {
	case event := <-fillChan:
		if event.Status != exchange.OrderStatusFilled {
			t.Errorf("expected FILLED status, got %s", event.Status)
		}
	default:
		t.Error("expected fill event in channel")
	}
}

func TestSimulateFills_SellFillsWhenPriceTouchesHigh(t *testing.T) {
	t.Parallel()

	m := New(map[string]exchange.Balance{
		"USDT": {Asset: "USDT", Free: 1000},
		"BTC":  {Asset: "BTC", Free: 0.1},
	}, 0.001)
	ctx := context.Background()
	m.SetPrice(90000)

	_, _ = m.PlaceLimitOrder(ctx, exchange.PlaceOrderRequest{
		Symbol:   "BTCUSDT",
		Side:     exchange.OrderSideSell,
		Price:    92000,
		Quantity: 0.01,
	})

	// Candle high reaches the sell price.
	candle := exchange.Candle{
		Open: 90000, High: 93000, Low: 89000, Close: 91000,
		CloseTime: time.Now(),
	}
	m.SimulateFills(candle)

	// USDT balance should have increased.
	bal, _ := m.GetBalance(ctx, "USDT")
	if bal.Free <= 1000 {
		t.Errorf("USDT balance should have increased after sell fill, got %.2f", bal.Free)
	}
}

func TestSimulateFills_NoFillOutsideRange(t *testing.T) {
	t.Parallel()

	m := newMock()
	ctx := context.Background()
	m.SetPrice(90000)

	_, _ = m.PlaceLimitOrder(ctx, exchange.PlaceOrderRequest{
		Symbol:   "BTCUSDT",
		Side:     exchange.OrderSideBuy,
		Price:    80000,
		Quantity: 0.01,
	})

	// Candle that doesn't reach 80000.
	candle := exchange.Candle{
		Open: 90000, High: 91000, Low: 88000, Close: 90500,
		CloseTime: time.Now(),
	}
	m.SimulateFills(candle)

	orders, _ := m.GetOpenOrders(ctx, "BTCUSDT")
	if len(orders) == 0 {
		t.Error("order should not have been filled — price didn't reach it")
	}
}

func TestCancelOrder(t *testing.T) {
	t.Parallel()

	m := newMock()
	ctx := context.Background()

	order, _ := m.PlaceLimitOrder(ctx, exchange.PlaceOrderRequest{
		Symbol: "BTCUSDT", Side: exchange.OrderSideBuy, Price: 80000, Quantity: 0.01,
	})

	if err := m.CancelOrder(ctx, "BTCUSDT", order.OrderID); err != nil {
		t.Fatalf("CancelOrder: %v", err)
	}

	// USDT should be unlocked.
	bal, _ := m.GetBalance(ctx, "USDT")
	if bal.Locked != 0 {
		t.Errorf("expected 0 locked USDT after cancel, got %.2f", bal.Locked)
	}
}

func TestTotalEquityUSDT(t *testing.T) {
	t.Parallel()

	m := New(map[string]exchange.Balance{
		"USDT": {Asset: "USDT", Free: 1000, Locked: 500},
		"BTC":  {Asset: "BTC", Free: 0.01, Locked: 0.005},
	}, 0)
	m.SetPrice(80000)

	// 1500 USDT + 0.015 BTC * 80000 = 1500 + 1200 = 2700
	equity := m.TotalEquityUSDT("BTC")
	want := 2700.0
	if equity != want {
		t.Errorf("got %.2f, want %.2f", equity, want)
	}
}

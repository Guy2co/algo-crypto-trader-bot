package risk

import (
	"context"
	"testing"

	"github.com/Guy2co/algo-crypto-trader-bot/internal/config"
	"github.com/Guy2co/algo-crypto-trader-bot/internal/exchange"
	"go.uber.org/zap"
)

func defaultConfig() config.RiskConfig {
	return config.RiskConfig{
		MaxPositionUSDT:   5000,
		StopLossPct:       5,
		MaxDrawdownPct:    15,
		MaxOpenOrders:     10,
		OrderCooldownSecs: 0, // disable cooldown in tests
		CancelOnStop:      false,
	}
}

func TestCheckOrderPlacement_Allowed(t *testing.T) {
	t.Parallel()

	m := New(defaultConfig(), zap.NewNop())
	result := m.CheckOrderPlacement(
		context.Background(),
		exchange.PlaceOrderRequest{Side: exchange.OrderSideBuy, Price: 90000, Quantity: 0.01},
		[]exchange.Order{},
		[]exchange.Balance{{Asset: "USDT", Free: 5000, Locked: 0}},
	)
	if !result.Allowed {
		t.Errorf("expected allowed, got reason: %s", result.Reason)
	}
}

func TestCheckOrderPlacement_MaxOpenOrders(t *testing.T) {
	t.Parallel()

	m := New(defaultConfig(), zap.NewNop())
	orders := make([]exchange.Order, 10)
	result := m.CheckOrderPlacement(
		context.Background(),
		exchange.PlaceOrderRequest{Side: exchange.OrderSideBuy, Price: 90000, Quantity: 0.01},
		orders,
		[]exchange.Balance{{Asset: "USDT", Free: 5000, Locked: 0}},
	)
	if result.Allowed {
		t.Error("expected blocked due to max open orders")
	}
}

func TestCheckOrderPlacement_MaxPosition(t *testing.T) {
	t.Parallel()

	m := New(defaultConfig(), zap.NewNop())
	// Locked = 4990, ordering 200 USDT more → total 5190 > 5000 max
	result := m.CheckOrderPlacement(
		context.Background(),
		exchange.PlaceOrderRequest{Side: exchange.OrderSideBuy, Price: 90000, Quantity: 0.1},
		[]exchange.Order{},
		[]exchange.Balance{{Asset: "USDT", Free: 100, Locked: 4990}},
	)
	if result.Allowed {
		t.Error("expected blocked due to max position")
	}
}

func TestCheckOrderPlacement_Halted(t *testing.T) {
	t.Parallel()

	m := New(defaultConfig(), zap.NewNop())
	m.Halt("test halt")
	result := m.CheckOrderPlacement(
		context.Background(),
		exchange.PlaceOrderRequest{},
		nil,
		nil,
	)
	if result.Allowed {
		t.Error("expected blocked when halted")
	}
}

func TestCheckStopLoss_PriceInRange(t *testing.T) {
	t.Parallel()

	m := New(defaultConfig(), zap.NewNop())
	result := m.CheckStopLoss(context.Background(), 90000, 80000, 100000)
	if !result.Allowed {
		t.Errorf("price in range should be allowed, got: %s", result.Reason)
	}
}

func TestCheckStopLoss_PriceBelowThreshold(t *testing.T) {
	t.Parallel()

	m := New(defaultConfig(), zap.NewNop())
	// Stop loss at 5% below 80000 = 76000. Price 74000 should trigger.
	result := m.CheckStopLoss(context.Background(), 74000, 80000, 100000)
	if result.Allowed {
		t.Error("expected stop loss to trigger")
	}
	if !m.Halted() {
		t.Error("expected manager to be halted after stop loss")
	}
}

func TestCheckStopLoss_PriceAboveThreshold(t *testing.T) {
	t.Parallel()

	m := New(defaultConfig(), zap.NewNop())
	// Stop loss at 5% above 100000 = 105000. Price 106000 should trigger.
	result := m.CheckStopLoss(context.Background(), 106000, 80000, 100000)
	if result.Allowed {
		t.Error("expected stop loss to trigger above range")
	}
}

func TestCheckDrawdown(t *testing.T) {
	t.Parallel()

	m := New(defaultConfig(), zap.NewNop())
	m.RecordEquity(10000)

	// 8% drawdown — should be allowed.
	result := m.CheckDrawdown(context.Background(), 9200)
	if !result.Allowed {
		t.Errorf("8%% drawdown should be allowed, got: %s", result.Reason)
	}

	// 20% drawdown — should halt.
	result = m.CheckDrawdown(context.Background(), 8000)
	if result.Allowed {
		t.Error("20%% drawdown should trigger halt")
	}
	if !m.Halted() {
		t.Error("manager should be halted after drawdown")
	}
}

func TestCalculateEquity(t *testing.T) {
	t.Parallel()

	balances := []exchange.Balance{
		{Asset: "USDT", Free: 500, Locked: 200},
		{Asset: "BTC", Free: 0.01, Locked: 0.005},
	}
	// Equity = 700 USDT + 0.015 BTC * 80000 = 700 + 1200 = 1900
	equity := CalculateEquity(balances, "BTC", 80000)
	want := 1900.0
	if equity != want {
		t.Errorf("got %.2f, want %.2f", equity, want)
	}
}

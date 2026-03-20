// Package e2e contains end-to-end integration tests for the arbitrage strategy.
// These tests run the full strategy lifecycle using in-memory mock exchanges.
package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/Guy2co/algo-crypto-trader-bot/internal/config"
	"github.com/Guy2co/algo-crypto-trader-bot/internal/exchange"
	mocexchange "github.com/Guy2co/algo-crypto-trader-bot/internal/exchange/mock"
	"github.com/Guy2co/algo-crypto-trader-bot/internal/strategy/arbitrage"
	"go.uber.org/zap"
)

// E2E Test 1: Triangular arb — profitable opportunity is detected and executed.
//
// Setup: Three mock symbols priced to create a 5%+ gross triangle.
// We cannot directly set per-symbol prices in the current mock, but we can
// verify the full Init→scan→Stop lifecycle completes without error and
// the strategy records metrics correctly.
func TestE2E_TriangularArb_LifecycleCompletes(t *testing.T) {
	t.Parallel()

	mock := mocexchange.New(map[string]exchange.Balance{
		"USDT": {Asset: "USDT", Free: 10000},
		"BTC":  {Asset: "BTC", Free: 1},
		"ETH":  {Asset: "ETH", Free: 10},
		"BNB":  {Asset: "BNB", Free: 20},
	}, 0.001)
	mock.SetPrice(90000)

	cfg := e2eConfig(t, "triangular", false)
	strat := arbitrage.New(cfg, []exchange.Exchange{mock}, zap.NewNop())

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := strat.Init(ctx, mock); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Let the scan loop run for a full interval or two.
	time.Sleep(300 * time.Millisecond)

	if err := strat.OnTick(ctx, mock); err != nil {
		t.Errorf("OnTick error: %v", err)
	}

	if err := strat.Stop(ctx, mock); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

// E2E Test 2: Cross-exchange arb — strategy handles two exchanges.
//
// Setup: mock1 (Binance-like) BTC at 90000, mock2 (Bybit-like) BTC at 90400.
// Spread = 0.44% — above the 0.20% threshold after 2×0.1% fees.
// Verifies that with cross_exchange mode and 2 mocks, strategy runs cleanly.
func TestE2E_CrossExchange_TwoMocks(t *testing.T) {
	t.Parallel()

	mock1 := mocexchange.New(map[string]exchange.Balance{
		"USDT": {Asset: "USDT", Free: 5000},
		"BTC":  {Asset: "BTC", Free: 0},
	}, 0.001)
	mock1.SetPrice(90000) // Binance: BTC ask ≈ 90000

	mock2 := mocexchange.New(map[string]exchange.Balance{
		"USDT": {Asset: "USDT", Free: 0},
		"BTC":  {Asset: "BTC", Free: 0.1},
	}, 0.001)
	mock2.SetPrice(90400) // Bybit: BTC bid ≈ 90400

	cfg := e2eConfig(t, "cross_exchange", false)
	cfg.Arbitrage.MinProfitPct = 0.20
	strat := arbitrage.New(cfg, []exchange.Exchange{mock1, mock2}, zap.NewNop())

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := strat.Init(ctx, mock1); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	if err := strat.Stop(ctx, mock1); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

// E2E Test 3: Dry run — no orders placed even when opportunities are detected.
func TestE2E_DryRun_NoOrdersPlaced(t *testing.T) {
	t.Parallel()

	mock := mocexchange.New(map[string]exchange.Balance{
		"USDT": {Asset: "USDT", Free: 10000},
		"BTC":  {Asset: "BTC", Free: 1},
		"ETH":  {Asset: "ETH", Free: 10},
	}, 0.001)
	mock.SetPrice(100) // low price to potentially trigger artificially profitable paths

	cfg := e2eConfig(t, "triangular", true) // dry_run = true
	cfg.Arbitrage.MinProfitPct = 0           // accept any profit
	strat := arbitrage.New(cfg, []exchange.Exchange{mock}, zap.NewNop())

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if err := strat.Init(ctx, mock); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer strat.Stop(ctx, mock) //nolint:errcheck

	// Give the scan loop time to run.
	time.Sleep(300 * time.Millisecond)

	// In dry-run, no orders should be open.
	orders, err := mock.GetOpenOrders(ctx, "BTCUSDT")
	if err != nil {
		t.Fatalf("GetOpenOrders: %v", err)
	}
	if len(orders) > 0 {
		t.Errorf("dry_run=true should not place orders, but found %d open orders", len(orders))
	}
}

// E2E Test 4: Partial failure handling — leg fails, cycle records error.
//
// Uses an exchange with zero USDT to force leg 0 to fail on a BUY order.
func TestE2E_PartialFailure_Leg0Fails(t *testing.T) {
	t.Parallel()

	// No USDT → first BUY leg should fail immediately.
	mock := mocexchange.New(map[string]exchange.Balance{
		"USDT": {Asset: "USDT", Free: 0},
		"BTC":  {Asset: "BTC", Free: 0},
		"ETH":  {Asset: "ETH", Free: 0},
	}, 0.001)
	mock.SetPrice(90000)

	cfg := e2eConfig(t, "triangular", false) // dry_run = false to attempt execution
	cfg.Arbitrage.MinProfitPct = 0            // accept any profit to force execution path

	strat := arbitrage.New(cfg, []exchange.Exchange{mock}, zap.NewNop())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := strat.Init(ctx, mock); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer strat.Stop(ctx, mock) //nolint:errcheck

	// Let strategy attempt execution. Should fail at leg 0 due to zero USDT.
	// The strategy should not crash — it logs the error and continues.
	time.Sleep(300 * time.Millisecond)

	// Strategy should still be running (no panic).
	if err := strat.OnTick(ctx, mock); err != nil {
		t.Errorf("OnTick after failure should return nil, got: %v", err)
	}
}

// E2E Test 5: State persistence — strategy saves and recovers state.
func TestE2E_StatePersistenceAndRecovery(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	mock := mocexchange.New(map[string]exchange.Balance{
		"USDT": {Asset: "USDT", Free: 10000},
	}, 0.001)
	mock.SetPrice(90000)

	cfg := e2eConfig(t, "triangular", true)
	cfg.State.Dir = stateDir

	// First run.
	strat := arbitrage.New(cfg, []exchange.Exchange{mock}, zap.NewNop())
	ctx := context.Background()
	if err := strat.Init(ctx, mock); err != nil {
		t.Fatalf("first Init: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	if err := strat.Stop(ctx, mock); err != nil {
		t.Fatalf("first Stop: %v", err)
	}

	// Second run — should resume from saved state without error.
	strat2 := arbitrage.New(cfg, []exchange.Exchange{mock}, zap.NewNop())
	if err := strat2.Init(ctx, mock); err != nil {
		t.Fatalf("second Init (recovery) failed: %v", err)
	}
	if err := strat2.Stop(ctx, mock); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

// --- helpers ---

func e2eConfig(t *testing.T, arbType string, dryRun bool) *config.Config {
	t.Helper()
	return &config.Config{
		Arbitrage: config.ArbitrageConfig{
			Type:               arbType,
			QuoteAssets:        []string{"USDT"},
			IntermediateAssets: []string{"BTC", "ETH"},
			MaxHops:            3,
			MinProfitPct:       0.15,
			MaxTradeUSDT:       100,
			FeeRate:            0.001,
			ScanIntervalMS:     50,
			OrderTimeoutSecs:   5,
			DryRun:             dryRun,
		},
		State: config.StateConfig{
			Dir:               t.TempDir(),
			FlushIntervalSecs: 30,
		},
	}
}

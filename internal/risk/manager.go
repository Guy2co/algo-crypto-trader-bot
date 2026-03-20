// Package risk implements pre-trade and portfolio-level risk controls.
package risk

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/Guy2co/algo-crypto-trader-bot/internal/config"
	"github.com/Guy2co/algo-crypto-trader-bot/internal/exchange"
	"go.uber.org/zap"
)

// CheckResult is returned by every risk check.
type CheckResult struct {
	Allowed bool
	Reason  string
}

// Manager implements all risk controls for the bot.
type Manager struct {
	mu            sync.Mutex
	cfg           config.RiskConfig
	peakEquity    float64
	lastOrderTime time.Time
	halted        bool
	logger        *zap.Logger
}

// New creates a new Manager.
func New(cfg config.RiskConfig, logger *zap.Logger) *Manager {
	return &Manager{cfg: cfg, logger: logger}
}

// Halted reports whether the risk manager has triggered an emergency halt.
func (m *Manager) Halted() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.halted
}

// Halt triggers an emergency halt and logs the reason.
func (m *Manager) Halt(reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.halted = true
	m.logger.Error("risk halt triggered", zap.String("reason", reason))
}

// RecordEquity updates the high-water mark used for drawdown calculation.
func (m *Manager) RecordEquity(equity float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.peakEquity = math.Max(m.peakEquity, equity)
}

// CheckOrderPlacement evaluates all pre-order risk controls.
func (m *Manager) CheckOrderPlacement(
	_ context.Context,
	req exchange.PlaceOrderRequest,
	openOrders []exchange.Order,
	balances []exchange.Balance,
) CheckResult {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.halted {
		return CheckResult{Allowed: false, Reason: "bot is halted"}
	}

	// Cooldown
	if m.cfg.OrderCooldownSecs > 0 {
		elapsed := time.Since(m.lastOrderTime)
		if elapsed < time.Duration(m.cfg.OrderCooldownSecs)*time.Second {
			return CheckResult{Allowed: false, Reason: "order cooldown active"}
		}
	}

	// Max open orders
	if len(openOrders) >= m.cfg.MaxOpenOrders {
		return CheckResult{Allowed: false, Reason: "max open orders reached"}
	}

	// Max position USDT — sum locked USDT across all buy orders
	if req.Side == exchange.OrderSideBuy {
		var lockedUSDT float64
		for _, b := range balances {
			if b.Asset == "USDT" {
				lockedUSDT = b.Locked
				break
			}
		}
		orderCost := req.Price * req.Quantity
		if lockedUSDT+orderCost > m.cfg.MaxPositionUSDT {
			return CheckResult{Allowed: false, Reason: "max position USDT exceeded"}
		}
	}

	m.lastOrderTime = time.Now()
	return CheckResult{Allowed: true}
}

// CheckStopLoss halts trading if price has moved beyond the acceptable range.
func (m *Manager) CheckStopLoss(_ context.Context, currentPrice, gridBottom, gridTop float64) CheckResult {
	m.mu.Lock()
	defer m.mu.Unlock()

	stopBottom := gridBottom * (1 - m.cfg.StopLossPct/100)
	stopTop := gridTop * (1 + m.cfg.StopLossPct/100)

	if currentPrice < stopBottom {
		m.halted = true
		reason := "price below stop-loss threshold"
		m.logger.Error("stop loss triggered", zap.Float64("price", currentPrice), zap.Float64("threshold", stopBottom))
		return CheckResult{Allowed: false, Reason: reason}
	}
	if currentPrice > stopTop {
		m.halted = true
		reason := "price above stop-loss threshold"
		m.logger.Error("stop loss triggered", zap.Float64("price", currentPrice), zap.Float64("threshold", stopTop))
		return CheckResult{Allowed: false, Reason: reason}
	}

	return CheckResult{Allowed: true}
}

// CheckDrawdown halts trading if the portfolio has lost too much from its peak.
func (m *Manager) CheckDrawdown(_ context.Context, currentEquity float64) CheckResult {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.peakEquity == 0 {
		return CheckResult{Allowed: true}
	}

	drawdown := (m.peakEquity - currentEquity) / m.peakEquity * 100
	if drawdown > m.cfg.MaxDrawdownPct {
		m.halted = true
		m.logger.Error("max drawdown triggered",
			zap.Float64("drawdown_pct", drawdown),
			zap.Float64("max_pct", m.cfg.MaxDrawdownPct),
		)
		return CheckResult{Allowed: false, Reason: "max drawdown exceeded"}
	}

	return CheckResult{Allowed: true}
}

// CalculateEquity computes total portfolio value in USDT.
func CalculateEquity(balances []exchange.Balance, baseAsset string, currentPrice float64) float64 {
	var equity float64
	for _, b := range balances {
		switch b.Asset {
		case "USDT":
			equity += b.Total()
		case baseAsset:
			equity += b.Total() * currentPrice
		}
	}
	return equity
}

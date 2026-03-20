// Package grid implements the Grid Trading strategy.
//
// Grid trading places buy orders at fixed price intervals below the current
// price and sell orders above. When a buy fills, a sell is placed one level
// higher; when a sell fills, a buy is placed one level lower. Profit is
// captured from price oscillation within the grid range.
package grid

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Guy2co/algo-crypto-trader-bot/internal/config"
	"github.com/Guy2co/algo-crypto-trader-bot/internal/exchange"
	"github.com/Guy2co/algo-crypto-trader-bot/internal/order"
	"github.com/Guy2co/algo-crypto-trader-bot/internal/strategy"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Strategy implements the grid trading strategy.
type Strategy struct {
	cfg      *config.Config
	state    *GridState
	tracker  *order.Tracker
	stateDir string
	logger   *zap.Logger
}

// New creates a new grid Strategy. Called by the strategy registry.
func New(cfg *config.Config, logger *zap.Logger) strategy.Strategy {
	return &Strategy{
		cfg:      cfg,
		tracker:  order.NewTracker(),
		stateDir: cfg.State.Dir,
		logger:   logger,
	}
}

// Name returns the strategy's registry key.
func (g *Strategy) Name() string { return "grid" }

// Init loads or creates the grid state, then places or recovers orders.
func (g *Strategy) Init(ctx context.Context, ex exchange.Exchange) error {
	if err := os.MkdirAll(g.stateDir, 0o750); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}

	existing, err := loadState(g.stateDir, g.cfg.Grid.Symbol)
	if err != nil {
		return fmt.Errorf("load grid state: %w", err)
	}

	if existing != nil {
		g.state = existing
		g.logger.Info("recovering grid",
			zap.String("run_id", g.state.RunID),
			zap.Int("levels", len(g.state.Levels)),
			zap.Int("cycles", g.state.Metrics.TotalCycles),
		)
		return g.recoverGrid(ctx, ex)
	}

	// Fresh start.
	currentPrice, err := ex.GetCurrentPrice(ctx, g.cfg.Grid.Symbol)
	if err != nil {
		return fmt.Errorf("get current price: %w", err)
	}

	g.state = &GridState{
		RunID:       uuid.New().String(),
		Symbol:      g.cfg.Grid.Symbol,
		GridBottom:  g.cfg.Grid.GridBottom,
		GridTop:     g.cfg.Grid.GridTop,
		GridCount:   g.cfg.Grid.GridCount,
		GridSpacing: (g.cfg.Grid.GridTop - g.cfg.Grid.GridBottom) / float64(g.cfg.Grid.GridCount),
		Investment:  g.cfg.Grid.TotalInvestment,
		Metrics:     GridMetrics{StartTime: time.Now()},
	}

	g.logger.Info("initializing grid",
		zap.String("symbol", g.state.Symbol),
		zap.Float64("bottom", g.state.GridBottom),
		zap.Float64("top", g.state.GridTop),
		zap.Int("count", g.state.GridCount),
		zap.Float64("current_price", currentPrice),
	)

	if err = g.initializeGrid(ctx, ex, currentPrice); err != nil {
		return fmt.Errorf("initialize grid orders: %w", err)
	}

	return g.state.save(g.stateDir)
}

// OnFill handles an order fill event from the exchange stream.
func (g *Strategy) OnFill(ctx context.Context, ex exchange.Exchange, event exchange.OrderFillEvent) error {
	if event.Status != exchange.OrderStatusFilled {
		return nil
	}
	if g.tracker.IsDuplicate(event.TradeID) {
		g.logger.Debug("duplicate fill event dropped", zap.Int64("trade_id", event.TradeID))
		return nil
	}

	if err := g.handleFill(ctx, ex, event); err != nil {
		return fmt.Errorf("handle fill: %w", err)
	}

	return g.state.save(g.stateDir)
}

// OnTick performs periodic housekeeping.
func (g *Strategy) OnTick(ctx context.Context, ex exchange.Exchange) error {
	price, err := ex.GetCurrentPrice(ctx, g.state.Symbol)
	if err != nil {
		g.logger.Warn("tick: failed to get price", zap.Error(err))
		return nil
	}

	g.state.mu.RLock()
	m := g.state.Metrics
	g.state.mu.RUnlock()

	g.logger.Info("grid tick",
		zap.Float64("price", price),
		zap.Int("cycles", m.TotalCycles),
		zap.Float64("profit_usdt", m.TotalProfit),
		zap.Float64("fees_paid", m.TotalFeesPaid),
	)

	return nil
}

// Stop gracefully shuts down the strategy.
func (g *Strategy) Stop(ctx context.Context, ex exchange.Exchange) error {
	g.logger.Info("stopping grid strategy")

	if g.cfg.Risk.CancelOnStop {
		if err := ex.CancelAllOrders(ctx, g.state.Symbol); err != nil {
			g.logger.Error("failed to cancel orders on stop", zap.Error(err))
		}
	}

	if err := g.state.save(g.stateDir); err != nil {
		return fmt.Errorf("save state on stop: %w", err)
	}

	g.state.mu.RLock()
	defer g.state.mu.RUnlock()

	g.logger.Info("grid strategy stopped",
		zap.Int("total_cycles", g.state.Metrics.TotalCycles),
		zap.Float64("total_profit_usdt", g.state.Metrics.TotalProfit),
	)
	return nil
}

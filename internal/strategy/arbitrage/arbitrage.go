// Package arbitrage implements triangular and cross-exchange arbitrage strategies.
//
// Supported modes:
//   - "triangular": Exploit cross-rate inefficiencies on a single exchange (3-hop or 4-hop paths).
//   - "cross_exchange": Buy cheap on one exchange, sell expensive on another.
//   - "both": Run both modes simultaneously.
package arbitrage

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Guy2co/algo-crypto-trader-bot/internal/config"
	"github.com/Guy2co/algo-crypto-trader-bot/internal/exchange"
	"github.com/Guy2co/algo-crypto-trader-bot/internal/strategy"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Strategy implements the arbitrage trading strategy.
type Strategy struct {
	cfg       *config.Config
	exchanges []exchange.Exchange
	paths     []pathTemplate
	state     *ArbState
	stateDir  string
	cancel    context.CancelFunc
	cycleNum  int
	logger    *zap.Logger
}

// New creates a new arbitrage Strategy with the given exchanges.
// exchanges[0] is always the primary (Binance); exchanges[1] is optional (Bybit).
func New(cfg *config.Config, exchanges []exchange.Exchange, logger *zap.Logger) strategy.Strategy {
	return &Strategy{
		cfg:       cfg,
		exchanges: exchanges,
		stateDir:  cfg.State.Dir,
		logger:    logger,
	}
}

// Name returns the strategy's registry key.
func (s *Strategy) Name() string { return "arbitrage" }

// Init initialises state, builds path templates, and starts the scan goroutine.
func (s *Strategy) Init(ctx context.Context, ex exchange.Exchange) error {
	// If created via registry (single exchange), set exchanges from Init argument.
	if len(s.exchanges) == 0 {
		s.exchanges = []exchange.Exchange{ex}
	}

	if err := os.MkdirAll(s.stateDir, 0o750); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}

	existing, err := loadArbState(s.stateDir)
	if err != nil {
		return fmt.Errorf("load arb state: %w", err)
	}

	if existing != nil {
		s.state = existing
		s.logger.Info("resuming arbitrage strategy",
			zap.String("run_id", s.state.RunID),
			zap.Int("prior_cycles", s.state.Metrics.TotalCycles),
		)
	} else {
		s.state = &ArbState{
			RunID:   uuid.New().String(),
			Metrics: ArbMetrics{StartTime: time.Now()},
		}
	}

	s.buildPaths()

	s.logger.Info("arbitrage strategy initialised",
		zap.String("type", s.cfg.Arbitrage.Type),
		zap.Int("paths", len(s.paths)),
		zap.Bool("dry_run", s.cfg.Arbitrage.DryRun),
	)

	scanCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	go s.scanLoop(scanCtx)

	return nil
}

func (s *Strategy) buildPaths() {
	cfg := s.cfg.Arbitrage
	quotes := cfg.QuoteAssets
	if len(quotes) == 0 {
		quotes = []string{"USDT"}
	}

	switch cfg.Type {
	case "triangular":
		s.paths = BuildTrianglePaths(quotes, cfg.IntermediateAssets, s.exchanges)
		if cfg.MaxHops >= 4 {
			s.paths = append(s.paths, BuildQuadPaths(quotes, cfg.IntermediateAssets, s.exchanges)...)
		}
	case "cross_exchange":
		s.paths = BuildCrossExchangePaths(quotes, cfg.IntermediateAssets, s.exchanges)
	case "both":
		s.paths = BuildTrianglePaths(quotes, cfg.IntermediateAssets, s.exchanges)
		if cfg.MaxHops >= 4 {
			s.paths = append(s.paths, BuildQuadPaths(quotes, cfg.IntermediateAssets, s.exchanges)...)
		}
		s.paths = append(s.paths, BuildCrossExchangePaths(quotes, cfg.IntermediateAssets, s.exchanges)...)
	default:
		// Default to triangular.
		s.paths = BuildTrianglePaths(quotes, cfg.IntermediateAssets, s.exchanges)
	}
}

func (s *Strategy) scanLoop(ctx context.Context) {
	interval := time.Duration(s.cfg.Arbitrage.ScanIntervalMS) * time.Millisecond
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.scan(ctx)
		}
	}
}

func (s *Strategy) scan(ctx context.Context) {
	cfg := s.cfg.Arbitrage
	for _, tmpl := range s.paths {
		path, profitable, err := EvaluatePath(ctx, tmpl, cfg.FeeRate, cfg.MinProfitPct)
		if err != nil {
			s.logger.Debug("path evaluation error", zap.Error(err))
			continue
		}

		s.state.mu.Lock()
		s.state.Metrics.TotalOpportunities++
		s.state.mu.Unlock()

		if !profitable {
			continue
		}

		s.logger.Info("arbitrage opportunity found",
			zap.String("type", path.PathType),
			zap.Float64("net_profit_pct", path.NetProfitPct),
			zap.Int("legs", len(path.Legs)),
		)

		if cfg.DryRun {
			continue
		}

		s.execute(ctx, path)
	}
}

func (s *Strategy) execute(ctx context.Context, path ArbPath) {
	s.cycleNum++
	orderCtx, cancel := context.WithTimeout(ctx, time.Duration(s.cfg.Arbitrage.OrderTimeoutSecs)*time.Second)
	defer cancel()

	result := ExecuteCycle(orderCtx, path, s.cfg.Arbitrage.MaxTradeUSDT, s.state.RunID, s.cycleNum, s.logger)

	s.state.mu.Lock()
	s.state.Metrics.TotalExecuted++
	if result.Err == nil {
		s.state.Metrics.TotalCycles++
		s.state.Metrics.TotalProfit += result.ActualProfit
		s.state.Metrics.LastCycleTime = time.Now()
		s.logger.Info("arbitrage cycle completed",
			zap.Float64("start_usdt", result.StartUSDT),
			zap.Float64("profit_usdt", result.ActualProfit),
			zap.Duration("duration", result.Duration),
		)
	} else {
		s.logger.Error("arbitrage cycle failed",
			zap.Int("failed_leg", result.FailedLeg),
			zap.Error(result.Err),
		)
	}
	s.state.mu.Unlock()

	if saveErr := s.state.save(s.stateDir); saveErr != nil {
		s.logger.Warn("failed to save arb state", zap.Error(saveErr))
	}
}

// OnFill is a no-op for the arbitrage strategy — execution is synchronous
// and does not rely on the fill event stream.
func (s *Strategy) OnFill(_ context.Context, _ exchange.Exchange, _ exchange.OrderFillEvent) error {
	return nil
}

// OnTick logs current performance metrics.
func (s *Strategy) OnTick(_ context.Context, _ exchange.Exchange) error {
	s.state.mu.RLock()
	m := s.state.Metrics
	s.state.mu.RUnlock()

	s.logger.Info("arbitrage tick",
		zap.Int("cycles", m.TotalCycles),
		zap.Int("executed", m.TotalExecuted),
		zap.Int("opportunities", m.TotalOpportunities),
		zap.Float64("profit_usdt", m.TotalProfit),
	)
	return nil
}

// Stop cancels the scan loop and saves state.
func (s *Strategy) Stop(_ context.Context, _ exchange.Exchange) error {
	s.logger.Info("stopping arbitrage strategy")
	if s.cancel != nil {
		s.cancel()
	}

	if err := s.state.save(s.stateDir); err != nil {
		return fmt.Errorf("save state on stop: %w", err)
	}

	s.state.mu.RLock()
	defer s.state.mu.RUnlock()

	s.logger.Info("arbitrage strategy stopped",
		zap.Int("total_cycles", s.state.Metrics.TotalCycles),
		zap.Float64("total_profit_usdt", s.state.Metrics.TotalProfit),
	)
	return nil
}

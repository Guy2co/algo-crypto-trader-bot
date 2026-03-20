// Package bot is the top-level coordinator that wires all components together
// and manages the live trading event loop.
package bot

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Guy2co/algo-crypto-trader-bot/internal/config"
	"github.com/Guy2co/algo-crypto-trader-bot/internal/exchange"
	"github.com/Guy2co/algo-crypto-trader-bot/internal/order"
	"github.com/Guy2co/algo-crypto-trader-bot/internal/risk"
	"github.com/Guy2co/algo-crypto-trader-bot/internal/strategy"
	"go.uber.org/zap"
)

// Bot is the top-level coordinator.
type Bot struct {
	cfg      *config.Config
	exchange exchange.Exchange
	strategy strategy.Strategy
	risk     *risk.Manager
	tracker  *order.Tracker
	logger   *zap.Logger
}

// New creates a new Bot.
func New(
	cfg *config.Config,
	ex exchange.Exchange,
	strat strategy.Strategy,
	riskMgr *risk.Manager,
	logger *zap.Logger,
) *Bot {
	return &Bot{
		cfg:      cfg,
		exchange: ex,
		strategy: strat,
		risk:     riskMgr,
		tracker:  order.NewTracker(),
		logger:   logger,
	}
}

// Run executes the full bot lifecycle until a signal or context cancellation.
func (b *Bot) Run(ctx context.Context) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	b.logger.Info("initializing strategy")
	if err := b.strategy.Init(ctx, b.exchange); err != nil {
		return fmt.Errorf("strategy init: %w", err)
	}

	primarySym := b.primarySymbol()
	b.logger.Info("subscribing to order fill stream", zap.String("symbol", primarySym))
	fillChan, cancelStream, err := b.exchange.SubscribeOrderFills(ctx, primarySym)
	if err != nil {
		return fmt.Errorf("subscribe order fills: %w", err)
	}
	defer cancelStream()

	// Seed equity baseline.
	if err = b.seedEquity(ctx, primarySym); err != nil {
		return err
	}

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	b.logger.Info("bot running — waiting for fills", zap.String("symbol", primarySym))

	for {
		select {
		case event, ok := <-fillChan:
			if !ok {
				return fmt.Errorf("fill channel closed unexpectedly")
			}
			if err = b.handleFillEvent(ctx, event); err != nil {
				b.logger.Error("fill event error", zap.Error(err))
				// Do not exit — log and continue to avoid missing fills.
			}

		case <-ticker.C:
			b.onTick(ctx)

		case <-ctx.Done():
			b.logger.Info("shutdown signal received — stopping strategy")
			b.stopStrategy()
			return nil
		}
	}
}

func (b *Bot) stopStrategy() {
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer stopCancel()
	if err := b.strategy.Stop(stopCtx, b.exchange); err != nil {
		b.logger.Error("strategy stop error", zap.Error(err))
	}
}

// primarySymbol returns the symbol used for fill subscriptions and equity tracking.
// For the grid strategy this is cfg.Grid.Symbol; for arbitrage the first quote pair.
func (b *Bot) primarySymbol() string {
	if b.cfg.Strategy.Active == "arbitrage" {
		quote := "USDT"
		if len(b.cfg.Arbitrage.QuoteAssets) > 0 {
			quote = b.cfg.Arbitrage.QuoteAssets[0]
		}
		intermediate := "BTC"
		if len(b.cfg.Arbitrage.IntermediateAssets) > 0 {
			intermediate = b.cfg.Arbitrage.IntermediateAssets[0]
		}
		return intermediate + quote
	}
	return b.cfg.Grid.Symbol
}

func (b *Bot) seedEquity(ctx context.Context, sym string) error {
	balances, err := b.exchange.GetBalances(ctx)
	if err != nil {
		return fmt.Errorf("get initial balances: %w", err)
	}
	price, err := b.exchange.GetCurrentPrice(ctx, sym)
	if err != nil {
		return fmt.Errorf("get initial price: %w", err)
	}
	equity := risk.CalculateEquity(balances, b.cfg.Grid.BaseAsset, price)
	b.risk.RecordEquity(equity)
	return nil
}

func (b *Bot) handleFillEvent(ctx context.Context, event exchange.OrderFillEvent) error {
	if event.Status != exchange.OrderStatusFilled {
		return nil
	}
	// For arbitrage the strategy handles its own fills; pass everything through.
	if b.cfg.Strategy.Active != "arbitrage" && event.Symbol != b.cfg.Grid.Symbol {
		return nil
	}
	if b.tracker.IsDuplicate(event.TradeID) {
		b.logger.Debug("duplicate fill dropped", zap.Int64("trade_id", event.TradeID))
		return nil
	}

	// Stop-loss check on fill price.
	result := b.risk.CheckStopLoss(ctx, event.Price, b.cfg.Grid.GridBottom, b.cfg.Grid.GridTop)
	if !result.Allowed {
		b.logger.Error("stop loss triggered on fill",
			zap.String("reason", result.Reason),
			zap.Float64("fill_price", event.Price),
		)
		return nil
	}

	return b.strategy.OnFill(ctx, b.exchange, event)
}

func (b *Bot) onTick(ctx context.Context) {
	if err := b.strategy.OnTick(ctx, b.exchange); err != nil {
		b.logger.Warn("strategy tick error", zap.Error(err))
	}

	balances, err := b.exchange.GetBalances(ctx)
	if err != nil {
		b.logger.Warn("tick: get balances error", zap.Error(err))
		return
	}
	primarySym := b.primarySymbol()
	price, err := b.exchange.GetCurrentPrice(ctx, primarySym)
	if err != nil {
		b.logger.Warn("tick: get price error", zap.Error(err))
		return
	}

	equity := risk.CalculateEquity(balances, b.cfg.Grid.BaseAsset, price)
	b.risk.RecordEquity(equity)

	result := b.risk.CheckDrawdown(ctx, equity)
	if !result.Allowed {
		b.logger.Error("max drawdown triggered", zap.String("reason", result.Reason))
	}

	result = b.risk.CheckStopLoss(ctx, price, b.cfg.Grid.GridBottom, b.cfg.Grid.GridTop)
	if !result.Allowed {
		b.logger.Error("stop loss on tick", zap.String("reason", result.Reason))
	}
}

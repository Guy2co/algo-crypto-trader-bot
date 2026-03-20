// Package register wires the arbitrage strategy into the global strategy registry.
// Import as a blank import in main packages to enable the "arbitrage" strategy
// with a single exchange (primary Binance only — no cross-exchange mode).
//
// For multi-exchange mode, use arbitrage.New(cfg, []exchange.Exchange{...}, logger)
// directly in cmd/bot/main.go.
package register

import (
	"github.com/Guy2co/algo-crypto-trader-bot/internal/config"
	"github.com/Guy2co/algo-crypto-trader-bot/internal/exchange"
	"github.com/Guy2co/algo-crypto-trader-bot/internal/strategy"
	"github.com/Guy2co/algo-crypto-trader-bot/internal/strategy/arbitrage"
	"go.uber.org/zap"
)

func init() {
	strategy.Register("arbitrage", func(cfg *config.Config, logger *zap.Logger) strategy.Strategy {
		// Single-exchange mode; Init() will set exchanges[0] from the Exchange passed to it.
		return arbitrage.New(cfg, []exchange.Exchange{}, logger)
	})
}

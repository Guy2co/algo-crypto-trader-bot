// Package register wires the grid strategy into the global strategy registry.
// Import it as a blank import in main packages to enable the "grid" strategy.
package register

import (
	"github.com/Guy2co/algo-crypto-trader-bot/internal/config"
	"github.com/Guy2co/algo-crypto-trader-bot/internal/strategy"
	"github.com/Guy2co/algo-crypto-trader-bot/internal/strategy/grid"
	"go.uber.org/zap"
)

func init() {
	strategy.Register("grid", func(cfg *config.Config, logger *zap.Logger) strategy.Strategy {
		return grid.New(cfg, logger)
	})
}

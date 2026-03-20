// Package register wires the grid strategy into the global strategy registry.
// Import it as a blank import in main packages to enable the "grid" strategy.
package register

import (
	"github.com/Guy2co/algo-crypto-trader-bot/internal/strategy"
	"github.com/Guy2co/algo-crypto-trader-bot/internal/strategy/grid"
)

func init() {
	strategy.Register("grid", grid.New)
}

package strategy

import (
	"fmt"

	"github.com/Guy2co/algo-crypto-trader-bot/internal/config"
	"go.uber.org/zap"
)

// Constructor is a factory function that creates a Strategy.
type Constructor func(cfg *config.Config, logger *zap.Logger) Strategy

var registry = map[string]Constructor{}

// Register adds a strategy constructor to the global registry.
// Call this from init() in each strategy package.
func Register(name string, fn Constructor) {
	registry[name] = fn
}

// New creates a strategy by name from the registry.
func New(name string, cfg *config.Config, logger *zap.Logger) (Strategy, error) {
	fn, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown strategy %q — register it with strategy.Register()", name)
	}
	return fn(cfg, logger), nil
}

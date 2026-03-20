// Package strategy defines the Strategy interface and registration mechanism.
package strategy

import (
	"context"

	"github.com/Guy2co/algo-crypto-trader-bot/internal/exchange"
)

// Strategy is the interface all trading strategies must implement.
type Strategy interface {
	// Name returns the strategy's human-readable identifier.
	Name() string

	// Init is called once at startup. The strategy must load persisted state
	// (if any), reconcile open orders with the exchange, and place initial
	// orders if starting fresh.
	Init(ctx context.Context, ex exchange.Exchange) error

	// OnFill is called by the bot whenever an order fill event arrives via
	// the WebSocket user data stream. This is the core rebalancing hook.
	OnFill(ctx context.Context, ex exchange.Exchange, event exchange.OrderFillEvent) error

	// OnTick is called periodically (every ~60s) for housekeeping: detecting
	// missed fills, logging metrics, and re-checking risk conditions.
	OnTick(ctx context.Context, ex exchange.Exchange) error

	// Stop is called on graceful shutdown. It should flush state to disk
	// and optionally cancel open orders.
	Stop(ctx context.Context, ex exchange.Exchange) error
}

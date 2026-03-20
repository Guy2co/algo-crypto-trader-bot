// Package exchange defines the Exchange interface and shared types used
// across all exchange implementations and strategies.
package exchange

import (
	"context"
	"time"
)

// OrderSide represents the direction of a trade.
type OrderSide string

const (
	OrderSideBuy  OrderSide = "BUY"
	OrderSideSell OrderSide = "SELL"
)

// OrderStatus mirrors the lifecycle of an exchange order.
type OrderStatus string

const (
	OrderStatusNew             OrderStatus = "NEW"
	OrderStatusPartiallyFilled OrderStatus = "PARTIALLY_FILLED"
	OrderStatusFilled          OrderStatus = "FILLED"
	OrderStatusCanceled        OrderStatus = "CANCELED"
	OrderStatusRejected        OrderStatus = "REJECTED"
)

// Order represents a resting or completed order on the exchange.
type Order struct {
	OrderID       int64
	ClientOrderID string
	Symbol        string
	Side          OrderSide
	Price         float64
	Quantity      float64
	FilledQty     float64
	Status        OrderStatus
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// PlaceOrderRequest holds parameters for placing a new limit order.
type PlaceOrderRequest struct {
	Symbol        string
	Side          OrderSide
	Price         float64
	Quantity      float64
	ClientOrderID string
}

// Balance holds the available and locked amounts for a single asset.
type Balance struct {
	Asset  string
	Free   float64
	Locked float64
}

// Total returns the sum of free and locked balance.
func (b Balance) Total() float64 {
	return b.Free + b.Locked
}

// BookTicker holds the best bid and ask for a symbol.
type BookTicker struct {
	Symbol   string
	BidPrice float64
	BidQty   float64
	AskPrice float64
	AskQty   float64
}

// MarketOrderRequest holds parameters for a market order.
type MarketOrderRequest struct {
	Symbol        string
	Side          OrderSide
	Quantity      float64
	ClientOrderID string
}

// Candle represents an OHLCV candlestick, used for backtesting.
type Candle struct {
	OpenTime  time.Time
	Open      float64
	High      float64
	Low       float64
	Close     float64
	Volume    float64
	CloseTime time.Time
}

// OrderFillEvent is emitted via the WebSocket user data stream when an
// order changes state (partial or full fill).
type OrderFillEvent struct {
	OrderID         int64
	ClientOrderID   string
	Symbol          string
	Side            OrderSide
	Price           float64
	Quantity        float64
	CumulativeQty   float64
	Status          OrderStatus
	Commission      float64
	CommissionAsset string
	EventTime       time.Time
	TradeID         int64
}

// Exchange is the primary abstraction over a trading venue.
// Implementations exist for Binance and as an in-memory mock for tests.
type Exchange interface {
	// Account
	GetBalances(ctx context.Context) ([]Balance, error)
	GetBalance(ctx context.Context, asset string) (Balance, error)

	// Orders
	PlaceLimitOrder(ctx context.Context, req PlaceOrderRequest) (*Order, error)
	CancelOrder(ctx context.Context, symbol string, orderID int64) error
	CancelAllOrders(ctx context.Context, symbol string) error
	GetOrder(ctx context.Context, symbol string, orderID int64) (*Order, error)
	GetOpenOrders(ctx context.Context, symbol string) ([]Order, error)

	// Market data
	GetCurrentPrice(ctx context.Context, symbol string) (float64, error)
	GetCandles(ctx context.Context, symbol, interval string, limit int) ([]Candle, error)

	// Streaming — returns a channel of fill events and a func to stop the stream.
	SubscribeOrderFills(ctx context.Context, symbol string) (<-chan OrderFillEvent, context.CancelFunc, error)

	// Arbitrage-specific
	GetBookTicker(ctx context.Context, symbol string) (BookTicker, error)
	PlaceMarketOrder(ctx context.Context, req MarketOrderRequest) (*Order, error)

	// Precision helpers
	FormatQuantity(symbol string, qty float64) (string, error)
	FormatPrice(symbol string, price float64) (string, error)
}

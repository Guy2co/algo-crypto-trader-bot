// Package bybit wraps the hirokisan/bybit SDK and implements the exchange.Exchange interface
// for Bybit Spot trading.
package bybit

import (
	"context"
	"fmt"
	"strconv"
	"time"

	gobybit "github.com/hirokisan/bybit/v2"
	"github.com/Guy2co/algo-crypto-trader-bot/internal/exchange"
	"go.uber.org/zap"
)

const (
	bybitTestnetBaseURL = "https://api-testnet.bybit.com"
	bybitLiveBaseURL    = "https://api.bybit.com"
	defaultTimeout      = 10 * time.Second
)

// Client implements exchange.Exchange for Bybit Spot.
type Client struct {
	inner  *gobybit.Client
	logger *zap.Logger
}

// NewClient creates a new Bybit client.
func NewClient(apiKey, secretKey string, testnet bool, logger *zap.Logger) (*Client, error) {
	baseURL := bybitLiveBaseURL
	if testnet {
		baseURL = bybitTestnetBaseURL
	}

	inner := gobybit.NewClient().
		WithAuth(apiKey, secretKey).
		WithBaseURL(baseURL)

	return &Client{inner: inner, logger: logger}, nil
}

// --- Account ---

func (c *Client) GetBalances(_ context.Context) ([]exchange.Balance, error) {
	resp, err := c.inner.Spot().V1().SpotGetWalletBalance()
	if err != nil {
		return nil, fmt.Errorf("GetBalances: %w", err)
	}
	balances := make([]exchange.Balance, 0, len(resp.Result.Balances))
	for _, b := range resp.Result.Balances {
		free, _ := strconv.ParseFloat(b.Free, 64)
		locked, _ := strconv.ParseFloat(b.Locked, 64)
		if free == 0 && locked == 0 {
			continue
		}
		balances = append(balances, exchange.Balance{
			Asset:  b.CoinName,
			Free:   free,
			Locked: locked,
		})
	}
	return balances, nil
}

func (c *Client) GetBalance(ctx context.Context, asset string) (exchange.Balance, error) {
	return exchange.GetBalanceSingle(ctx, c, asset)
}

// --- Orders ---

func (c *Client) PlaceLimitOrder(_ context.Context, req exchange.PlaceOrderRequest) (*exchange.Order, error) {
	side := gobybit.Side(req.Side)
	orderType := gobybit.OrderTypeSpotLimit
	tif := gobybit.TimeInForceSpotGTC
	linkID := req.ClientOrderID

	resp, err := c.inner.Spot().V1().SpotPostOrder(gobybit.SpotPostOrderParam{
		Symbol:      gobybit.SymbolSpot(req.Symbol),
		Qty:         req.Quantity,
		Side:        side,
		Type:        orderType,
		TimeInForce: &tif,
		Price:       &req.Price,
		OrderLinkID: &linkID,
	})
	if err != nil {
		return nil, fmt.Errorf("PlaceLimitOrder: %w", err)
	}
	return mapSpotOrder(resp.Result), nil
}

func (c *Client) PlaceMarketOrder(_ context.Context, req exchange.MarketOrderRequest) (*exchange.Order, error) {
	side := gobybit.Side(req.Side)
	orderType := gobybit.OrderTypeSpotMarket
	linkID := req.ClientOrderID

	resp, err := c.inner.Spot().V1().SpotPostOrder(gobybit.SpotPostOrderParam{
		Symbol:      gobybit.SymbolSpot(req.Symbol),
		Qty:         req.Quantity,
		Side:        side,
		Type:        orderType,
		OrderLinkID: &linkID,
	})
	if err != nil {
		return nil, fmt.Errorf("PlaceMarketOrder: %w", err)
	}
	return mapSpotOrder(resp.Result), nil
}

func (c *Client) CancelOrder(_ context.Context, _ string, orderID int64) error {
	id := strconv.FormatInt(orderID, 10)
	_, err := c.inner.Spot().V1().SpotDeleteOrder(gobybit.SpotDeleteOrderParam{
		OrderID: &id,
	})
	if err != nil {
		return fmt.Errorf("CancelOrder: %w", err)
	}
	return nil
}

func (c *Client) CancelAllOrders(_ context.Context, symbol string) error {
	_, err := c.inner.Spot().V1().SpotOrderBatchCancel(gobybit.SpotOrderBatchCancelParam{
		Symbol: gobybit.SymbolSpot(symbol),
	})
	if err != nil {
		return fmt.Errorf("CancelAllOrders: %w", err)
	}
	return nil
}

func (c *Client) GetOrder(_ context.Context, symbol string, orderID int64) (*exchange.Order, error) {
	id := strconv.FormatInt(orderID, 10)
	resp, err := c.inner.Spot().V1().SpotGetOrder(gobybit.SpotGetOrderParam{
		OrderID: &id,
	})
	if err != nil {
		return nil, fmt.Errorf("GetOrder: %w", err)
	}
	return mapSpotGetOrder(resp.Result), nil
}

func (c *Client) GetOpenOrders(_ context.Context, symbol string) ([]exchange.Order, error) {
	sym := gobybit.SymbolSpot(symbol)
	resp, err := c.inner.Spot().V1().SpotOpenOrders(gobybit.SpotOpenOrdersParam{
		Symbol: &sym,
	})
	if err != nil {
		return nil, fmt.Errorf("GetOpenOrders: %w", err)
	}
	orders := make([]exchange.Order, 0, len(resp.Result))
	for _, o := range resp.Result {
		orders = append(orders, *mapSpotOpenOrder(o))
	}
	return orders, nil
}

// --- Market data ---

func (c *Client) GetCurrentPrice(_ context.Context, symbol string) (float64, error) {
	sym := gobybit.SymbolSpot(symbol)
	resp, err := c.inner.Spot().V1().SpotQuoteTickerPrice(gobybit.SpotQuoteTickerPriceParam{
		Symbol: &sym,
	})
	if err != nil {
		return 0, fmt.Errorf("GetCurrentPrice: %w", err)
	}
	return strconv.ParseFloat(resp.Result.Price, 64)
}

func (c *Client) GetBookTicker(_ context.Context, symbol string) (exchange.BookTicker, error) {
	sym := gobybit.SymbolSpot(symbol)
	resp, err := c.inner.Spot().V1().SpotQuoteTickerBookTicker(gobybit.SpotQuoteTickerBookTickerParam{
		Symbol: &sym,
	})
	if err != nil {
		return exchange.BookTicker{}, fmt.Errorf("GetBookTicker: %w", err)
	}
	r := resp.Result
	bidPrice, _ := strconv.ParseFloat(r.BidPrice, 64)
	bidQty, _ := strconv.ParseFloat(r.BidQty, 64)
	askPrice, _ := strconv.ParseFloat(r.AskPrice, 64)
	askQty, _ := strconv.ParseFloat(r.AskQty, 64)
	return exchange.BookTicker{
		Symbol:   r.Symbol,
		BidPrice: bidPrice,
		BidQty:   bidQty,
		AskPrice: askPrice,
		AskQty:   askQty,
	}, nil
}

func (c *Client) GetCandles(_ context.Context, _ string, _ string, _ int) ([]exchange.Candle, error) {
	// Bybit candle fetching not needed for arbitrage strategy; stub returns empty slice.
	return nil, nil
}

// --- Streaming ---

// SubscribeOrderFills on Bybit uses REST polling as a minimal implementation.
// For production use, replace with WebSocket subscription.
func (c *Client) SubscribeOrderFills(ctx context.Context, symbol string) (<-chan exchange.OrderFillEvent, context.CancelFunc, error) {
	ch := make(chan exchange.OrderFillEvent, 64)
	cancelCtx, cancel := context.WithCancel(ctx)

	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-cancelCtx.Done():
				close(ch)
				return
			case <-ticker.C:
				c.pollFills(cancelCtx, symbol, ch)
			}
		}
	}()

	return ch, cancel, nil
}

func (c *Client) pollFills(_ context.Context, symbol string, _ chan<- exchange.OrderFillEvent) {
	// REST-based fill polling — logs only, does not send events for now.
	// Full WebSocket implementation can replace this in a future iteration.
	c.logger.Debug("bybit fill poll", zap.String("symbol", symbol))
}

// --- Precision ---

func (c *Client) FormatQuantity(_ string, qty float64) (string, error) {
	return strconv.FormatFloat(qty, 'f', 6, 64), nil
}

func (c *Client) FormatPrice(_ string, price float64) (string, error) {
	return strconv.FormatFloat(price, 'f', 2, 64), nil
}

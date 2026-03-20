package binance

import (
	"context"
	"fmt"
	"strconv"
	"time"

	gobinance "github.com/adshao/go-binance/v2"
	"github.com/Guy2co/algo-crypto-trader-bot/internal/exchange"
	"go.uber.org/zap"
)

const (
	defaultRESTTimeout = 10 * time.Second
	maxRetries         = 3
)

// Client wraps the go-binance SDK and implements exchange.Exchange.
type Client struct {
	inner      *gobinance.Client
	symbolInfo map[string]gobinance.Symbol
	logger     *zap.Logger
}

// NewClient creates a new Binance client.
// Set testnet=true to use the Binance testnet (testnet.binance.vision).
func NewClient(apiKey, secretKey string, testnet bool, logger *zap.Logger) (*Client, error) {
	if testnet {
		gobinance.UseTestnet = true
	}
	inner := gobinance.NewClient(apiKey, secretKey)
	inner.HTTPClient.Timeout = defaultRESTTimeout

	c := &Client{inner: inner, logger: logger, symbolInfo: make(map[string]gobinance.Symbol)}

	ctx, cancel := context.WithTimeout(context.Background(), defaultRESTTimeout)
	defer cancel()
	if err := c.loadSymbolInfo(ctx); err != nil {
		return nil, fmt.Errorf("load symbol info: %w", err)
	}
	return c, nil
}

func (c *Client) loadSymbolInfo(ctx context.Context) error {
	info, err := c.inner.NewExchangeInfoService().Do(ctx)
	if err != nil {
		return fmt.Errorf("get exchange info: %w", err)
	}
	for _, s := range info.Symbols {
		c.symbolInfo[s.Symbol] = s
	}
	return nil
}

// --- Account ---

func (c *Client) GetBalances(ctx context.Context) ([]exchange.Balance, error) {
	acct, err := c.inner.NewGetAccountService().Do(ctx)
	if err != nil {
		return nil, wrapAPIError("GetBalances", err)
	}
	balances := make([]exchange.Balance, 0, len(acct.Balances))
	for _, b := range acct.Balances {
		free, _ := strconv.ParseFloat(b.Free, 64)
		locked, _ := strconv.ParseFloat(b.Locked, 64)
		if free == 0 && locked == 0 {
			continue
		}
		balances = append(balances, exchange.Balance{
			Asset:  b.Asset,
			Free:   free,
			Locked: locked,
		})
	}
	return balances, nil
}

func (c *Client) GetBalance(ctx context.Context, asset string) (exchange.Balance, error) {
	balances, err := c.GetBalances(ctx)
	if err != nil {
		return exchange.Balance{}, err
	}
	for _, b := range balances {
		if b.Asset == asset {
			return b, nil
		}
	}
	return exchange.Balance{Asset: asset}, nil
}

// --- Orders ---

func (c *Client) PlaceLimitOrder(ctx context.Context, req exchange.PlaceOrderRequest) (*exchange.Order, error) {
	priceStr, err := c.FormatPrice(req.Symbol, req.Price)
	if err != nil {
		return nil, err
	}
	qtyStr, err := c.FormatQuantity(req.Symbol, req.Quantity)
	if err != nil {
		return nil, err
	}

	var resp *gobinance.CreateOrderResponse
	for attempt := range maxRetries {
		resp, err = c.inner.NewCreateOrderService().
			Symbol(req.Symbol).
			Side(gobinance.SideType(req.Side)).
			Type(gobinance.OrderTypeLimit).
			TimeInForce(gobinance.TimeInForceTypeGTC).
			Price(priceStr).
			Quantity(qtyStr).
			NewClientOrderID(req.ClientOrderID).
			Do(ctx)
		if err == nil {
			break
		}
		// Order already exists — treat as success, fetch the live order.
		if isOrderAlreadyExists(err) {
			c.logger.Debug("order already exists, skipping placement",
				zap.String("client_order_id", req.ClientOrderID),
			)
			return c.findOrderByClientID(ctx, req.Symbol, req.ClientOrderID)
		}
		if attempt < maxRetries-1 {
			time.Sleep(time.Duration(1<<uint(attempt)) * 100 * time.Millisecond)
		}
	}
	if err != nil {
		return nil, wrapAPIError("PlaceLimitOrder", err)
	}
	return mapCreateOrder(resp)
}

func (c *Client) findOrderByClientID(ctx context.Context, symbol, clientOrderID string) (*exchange.Order, error) {
	orders, err := c.GetOpenOrders(ctx, symbol)
	if err != nil {
		return nil, err
	}
	for _, o := range orders {
		if o.ClientOrderID == clientOrderID {
			return &o, nil
		}
	}
	return nil, fmt.Errorf("order with ClientOrderID %s not found", clientOrderID)
}

func (c *Client) CancelOrder(ctx context.Context, symbol string, orderID int64) error {
	_, err := c.inner.NewCancelOrderService().Symbol(symbol).OrderID(orderID).Do(ctx)
	if err != nil {
		return wrapAPIError("CancelOrder", err)
	}
	return nil
}

func (c *Client) CancelAllOrders(ctx context.Context, symbol string) error {
	_, err := c.inner.NewCancelOpenOrdersService().Symbol(symbol).Do(ctx)
	if err != nil {
		return wrapAPIError("CancelAllOrders", err)
	}
	return nil
}

func (c *Client) GetOrder(ctx context.Context, symbol string, orderID int64) (*exchange.Order, error) {
	o, err := c.inner.NewGetOrderService().Symbol(symbol).OrderID(orderID).Do(ctx)
	if err != nil {
		return nil, wrapAPIError("GetOrder", err)
	}
	return mapOrder(o)
}

func (c *Client) GetOpenOrders(ctx context.Context, symbol string) ([]exchange.Order, error) {
	orders, err := c.inner.NewListOpenOrdersService().Symbol(symbol).Do(ctx)
	if err != nil {
		return nil, wrapAPIError("GetOpenOrders", err)
	}
	result := make([]exchange.Order, 0, len(orders))
	for _, o := range orders {
		mapped, mapErr := mapOrder(o)
		if mapErr != nil {
			c.logger.Warn("failed to map order", zap.Int64("order_id", o.OrderID), zap.Error(mapErr))
			continue
		}
		result = append(result, *mapped)
	}
	return result, nil
}

// --- Market data ---

func (c *Client) GetCurrentPrice(ctx context.Context, symbol string) (float64, error) {
	prices, err := c.inner.NewListPricesService().Symbol(symbol).Do(ctx)
	if err != nil {
		return 0, wrapAPIError("GetCurrentPrice", err)
	}
	if len(prices) == 0 {
		return 0, fmt.Errorf("no price data for %s", symbol)
	}
	return strconv.ParseFloat(prices[0].Price, 64)
}

func (c *Client) GetCandles(ctx context.Context, symbol, interval string, limit int) ([]exchange.Candle, error) {
	klines, err := c.inner.NewKlinesService().Symbol(symbol).Interval(interval).Limit(limit).Do(ctx)
	if err != nil {
		return nil, wrapAPIError("GetCandles", err)
	}
	candles := make([]exchange.Candle, 0, len(klines))
	for _, k := range klines {
		open, _ := strconv.ParseFloat(k.Open, 64)
		high, _ := strconv.ParseFloat(k.High, 64)
		low, _ := strconv.ParseFloat(k.Low, 64)
		close_, _ := strconv.ParseFloat(k.Close, 64)
		volume, _ := strconv.ParseFloat(k.Volume, 64)
		candles = append(candles, exchange.Candle{
			OpenTime:  time.UnixMilli(k.OpenTime),
			Open:      open,
			High:      high,
			Low:       low,
			Close:     close_,
			Volume:    volume,
			CloseTime: time.UnixMilli(k.CloseTime),
		})
	}
	return candles, nil
}

// --- Precision ---

func (c *Client) FormatPrice(symbol string, price float64) (string, error) {
	info, ok := c.symbolInfo[symbol]
	if !ok {
		return strconv.FormatFloat(price, 'f', 2, 64), nil
	}
	for _, f := range info.Filters {
		if f["filterType"] == "PRICE_FILTER" {
			tickStr, _ := f["tickSize"].(string)
			if tickStr != "" {
				tick, err := strconv.ParseFloat(tickStr, 64)
				if err == nil && tick > 0 {
					rounded := roundToTick(price, tick)
					decimals := countDecimals(tickStr)
					return strconv.FormatFloat(rounded, 'f', decimals, 64), nil
				}
			}
		}
	}
	return strconv.FormatFloat(price, 'f', 2, 64), nil
}

func (c *Client) FormatQuantity(symbol string, qty float64) (string, error) {
	info, ok := c.symbolInfo[symbol]
	if !ok {
		return strconv.FormatFloat(qty, 'f', 8, 64), nil
	}
	for _, f := range info.Filters {
		if f["filterType"] == "LOT_SIZE" {
			stepStr, _ := f["stepSize"].(string)
			if stepStr != "" {
				step, err := strconv.ParseFloat(stepStr, 64)
				if err == nil && step > 0 {
					rounded := roundToStep(qty, step)
					decimals := countDecimals(stepStr)
					return strconv.FormatFloat(rounded, 'f', decimals, 64), nil
				}
			}
		}
	}
	return strconv.FormatFloat(qty, 'f', 8, 64), nil
}

func (c *Client) GetBookTicker(ctx context.Context, symbol string) (exchange.BookTicker, error) {
	tickers, err := c.inner.NewListBookTickersService().Symbol(symbol).Do(ctx)
	if err != nil {
		return exchange.BookTicker{}, wrapAPIError("GetBookTicker", err)
	}
	if len(tickers) == 0 {
		return exchange.BookTicker{}, fmt.Errorf("no book ticker for %s", symbol)
	}
	t := tickers[0]
	bidPrice, _ := strconv.ParseFloat(t.BidPrice, 64)
	bidQty, _ := strconv.ParseFloat(t.BidQuantity, 64)
	askPrice, _ := strconv.ParseFloat(t.AskPrice, 64)
	askQty, _ := strconv.ParseFloat(t.AskQuantity, 64)
	return exchange.BookTicker{
		Symbol:   t.Symbol,
		BidPrice: bidPrice,
		BidQty:   bidQty,
		AskPrice: askPrice,
		AskQty:   askQty,
	}, nil
}

func (c *Client) PlaceMarketOrder(ctx context.Context, req exchange.MarketOrderRequest) (*exchange.Order, error) {
	qtyStr, err := c.FormatQuantity(req.Symbol, req.Quantity)
	if err != nil {
		return nil, err
	}
	resp, err := c.inner.NewCreateOrderService().
		Symbol(req.Symbol).
		Side(gobinance.SideType(req.Side)).
		Type(gobinance.OrderTypeMarket).
		Quantity(qtyStr).
		NewClientOrderID(req.ClientOrderID).
		Do(ctx)
	if err != nil {
		return nil, wrapAPIError("PlaceMarketOrder", err)
	}
	return mapCreateOrder(resp)
}

func roundToTick(price, tick float64) float64 {
	if tick == 0 {
		return price
	}
	return float64(int64(price/tick)) * tick
}

func roundToStep(qty, step float64) float64 {
	if step == 0 {
		return qty
	}
	return float64(int64(qty/step)) * step
}

func countDecimals(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '.' {
			return len(s) - i - 1
		}
	}
	return 0
}

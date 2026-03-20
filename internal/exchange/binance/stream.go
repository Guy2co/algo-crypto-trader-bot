package binance

import (
	"context"
	"time"

	gobinance "github.com/adshao/go-binance/v2"
	"github.com/Guy2co/algo-crypto-trader-bot/internal/exchange"
	"go.uber.org/zap"
)

const (
	keepAliveInterval  = 30 * time.Minute
	reconnectBaseDelay = 100 * time.Millisecond
	reconnectMaxDelay  = 30 * time.Second
)

// SubscribeOrderFills opens the Binance user data stream and forwards FILLED
// order events to the returned channel. The returned CancelFunc stops the stream.
func (c *Client) SubscribeOrderFills(ctx context.Context, _ string) (<-chan exchange.OrderFillEvent, context.CancelFunc, error) {
	listenKey, err := c.inner.NewStartUserStreamService().Do(ctx)
	if err != nil {
		return nil, nil, wrapAPIError("StartUserStream", err)
	}

	outCh := make(chan exchange.OrderFillEvent, 256)
	streamCtx, streamCancel := context.WithCancel(ctx)

	// Keep-alive goroutine.
	go c.keepAliveListenKey(streamCtx, listenKey)

	// Stream goroutine.
	go c.runStream(streamCtx, listenKey, outCh)

	cancelFn := func() {
		streamCancel()
		_ = c.inner.NewCloseUserStreamService().ListenKey(listenKey).Do(context.Background())
		close(outCh)
	}

	return outCh, cancelFn, nil
}

func (c *Client) keepAliveListenKey(ctx context.Context, listenKey string) {
	ticker := time.NewTicker(keepAliveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := c.inner.NewKeepaliveUserStreamService().ListenKey(listenKey).Do(ctx); err != nil {
				c.logger.Warn("listenKey keepalive failed", zap.Error(err))
			}
		case <-ctx.Done():
			return
		}
	}
}

func (c *Client) runStream(ctx context.Context, listenKey string, outCh chan<- exchange.OrderFillEvent) {
	delay := reconnectBaseDelay
	for {
		if ctx.Err() != nil {
			return
		}

		doneC, stopC, err := gobinance.WsUserDataServe(listenKey, c.makeFillHandler(outCh), c.wsErrHandler())
		if err != nil {
			c.logger.Error("ws user data connect failed", zap.Error(err), zap.Duration("retry_in", delay))
			if !c.sleepOrCancel(ctx, delay) {
				return
			}
			delay = min(delay*2, reconnectMaxDelay)
			continue
		}

		c.logger.Info("ws user data stream connected")
		delay = reconnectBaseDelay // reset on success

		select {
		case <-ctx.Done():
			stopC <- struct{}{}
			return
		case <-doneC:
			c.logger.Warn("ws user data stream disconnected — reconnecting", zap.Duration("delay", delay))
			if !c.sleepOrCancel(ctx, delay) {
				return
			}
			delay = min(delay*2, reconnectMaxDelay)
		}
	}
}

// makeFillHandler returns the WsUserDataServe handler that forwards fills to outCh.
func (c *Client) makeFillHandler(outCh chan<- exchange.OrderFillEvent) func(*gobinance.WsUserDataEvent) {
	return func(event *gobinance.WsUserDataEvent) {
		if event.Event != gobinance.UserDataEventTypeExecutionReport {
			return
		}
		orderUpdate := event.OrderUpdate
		fill := mapWsOrderUpdate(&orderUpdate)
		if fill.Status != exchange.OrderStatusFilled && fill.Status != exchange.OrderStatusPartiallyFilled {
			return
		}
		select {
		case outCh <- fill:
		default:
			c.logger.Warn("fill channel full, dropping event", zap.Int64("order_id", fill.OrderID))
		}
	}
}

// wsErrHandler returns the WsUserDataServe error handler.
func (c *Client) wsErrHandler() func(error) {
	return func(err error) {
		c.logger.Warn("ws user data stream error", zap.Error(err))
	}
}

// sleepOrCancel waits for the given duration or until ctx is cancelled.
// Returns true if the sleep completed, false if the context was cancelled.
func (c *Client) sleepOrCancel(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

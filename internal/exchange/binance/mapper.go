package binance

import (
	"strconv"
	"time"

	gobinance "github.com/adshao/go-binance/v2"
	"github.com/Guy2co/algo-crypto-trader-bot/internal/exchange"
)

func mapOrder(o *gobinance.Order) (*exchange.Order, error) {
	price, err := strconv.ParseFloat(o.Price, 64)
	if err != nil {
		return nil, err
	}
	qty, err := strconv.ParseFloat(o.OrigQuantity, 64)
	if err != nil {
		return nil, err
	}
	filledQty, err := strconv.ParseFloat(o.ExecutedQuantity, 64)
	if err != nil {
		return nil, err
	}
	return &exchange.Order{
		OrderID:       o.OrderID,
		ClientOrderID: o.ClientOrderID,
		Symbol:        o.Symbol,
		Side:          exchange.OrderSide(o.Side),
		Price:         price,
		Quantity:      qty,
		FilledQty:     filledQty,
		Status:        exchange.OrderStatus(o.Status),
		CreatedAt:     time.UnixMilli(o.Time),
		UpdatedAt:     time.UnixMilli(o.UpdateTime),
	}, nil
}

func mapCreateOrder(o *gobinance.CreateOrderResponse) (*exchange.Order, error) {
	price, _ := strconv.ParseFloat(o.Price, 64)
	qty, _ := strconv.ParseFloat(o.OrigQuantity, 64)
	filledQty, _ := strconv.ParseFloat(o.ExecutedQuantity, 64)
	return &exchange.Order{
		OrderID:       o.OrderID,
		ClientOrderID: o.ClientOrderID,
		Symbol:        o.Symbol,
		Side:          exchange.OrderSide(o.Side),
		Price:         price,
		Quantity:      qty,
		FilledQty:     filledQty,
		Status:        exchange.OrderStatus(o.Status),
		CreatedAt:     time.UnixMilli(o.TransactTime),
		UpdatedAt:     time.UnixMilli(o.TransactTime),
	}, nil
}

func mapWsOrderUpdate(e *gobinance.WsOrderUpdate) exchange.OrderFillEvent {
	price, _ := strconv.ParseFloat(e.LatestPrice, 64)
	qty, _ := strconv.ParseFloat(e.LatestVolume, 64)
	cumQty, _ := strconv.ParseFloat(e.FilledVolume, 64)
	fee, _ := strconv.ParseFloat(e.FeeCost, 64)

	return exchange.OrderFillEvent{
		OrderID:         e.Id,
		ClientOrderID:   e.ClientOrderId,
		Symbol:          e.Symbol,
		Side:            exchange.OrderSide(e.Side),
		Price:           price,
		Quantity:        qty,
		CumulativeQty:   cumQty,
		Status:          exchange.OrderStatus(e.Status),
		Commission:      fee,
		CommissionAsset: e.FeeAsset,
		EventTime:       time.UnixMilli(e.TransactionTime),
		TradeID:         e.TradeId,
	}
}

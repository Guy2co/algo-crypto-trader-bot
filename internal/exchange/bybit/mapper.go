package bybit

import (
	"strconv"
	"time"

	gobybit "github.com/hirokisan/bybit/v2"
	"github.com/Guy2co/algo-crypto-trader-bot/internal/exchange"
)

func mapSpotOrder(r gobybit.SpotPostOrderResult) *exchange.Order {
	price, _ := strconv.ParseFloat(r.Price, 64)
	qty, _ := strconv.ParseFloat(r.OrigQty, 64)
	filledQty, _ := strconv.ParseFloat(r.ExecutedQty, 64)
	createdAt, _ := strconv.ParseInt(r.TransactTime, 10, 64)
	orderID, _ := strconv.ParseInt(r.OrderID, 10, 64)

	return &exchange.Order{
		OrderID:       orderID,
		ClientOrderID: r.OrderLinkID,
		Symbol:        r.Symbol,
		Side:          exchange.OrderSide(r.Side),
		Price:         price,
		Quantity:      qty,
		FilledQty:     filledQty,
		Status:        mapOrderStatus(string(r.Status)),
		CreatedAt:     time.UnixMilli(createdAt),
		UpdatedAt:     time.UnixMilli(createdAt),
	}
}

func mapSpotGetOrder(r gobybit.SpotGetOrderResult) *exchange.Order {
	price, _ := strconv.ParseFloat(r.Price, 64)
	qty, _ := strconv.ParseFloat(r.OrigQty, 64)
	filledQty, _ := strconv.ParseFloat(r.ExecutedQty, 64)
	createdAt, _ := strconv.ParseInt(r.Time, 10, 64)
	updatedAt, _ := strconv.ParseInt(r.UpdateTime, 10, 64)
	orderID, _ := strconv.ParseInt(r.OrderID, 10, 64)

	return &exchange.Order{
		OrderID:       orderID,
		ClientOrderID: r.OrderLinkID,
		Symbol:        r.Symbol,
		Side:          exchange.OrderSide(r.Side),
		Price:         price,
		Quantity:      qty,
		FilledQty:     filledQty,
		Status:        mapOrderStatus(string(r.Status)),
		CreatedAt:     time.UnixMilli(createdAt),
		UpdatedAt:     time.UnixMilli(updatedAt),
	}
}

func mapSpotOpenOrder(r gobybit.SpotOpenOrdersResult) *exchange.Order {
	price, _ := strconv.ParseFloat(r.Price, 64)
	qty, _ := strconv.ParseFloat(r.OrigQty, 64)
	filledQty, _ := strconv.ParseFloat(r.ExecutedQty, 64)
	createdAt, _ := strconv.ParseInt(r.Time, 10, 64)
	updatedAt, _ := strconv.ParseInt(r.UpdateTime, 10, 64)
	orderID, _ := strconv.ParseInt(r.OrderID, 10, 64)

	return &exchange.Order{
		OrderID:       orderID,
		ClientOrderID: r.OrderLinkID,
		Symbol:        r.Symbol,
		Side:          exchange.OrderSide(r.Side),
		Price:         price,
		Quantity:      qty,
		FilledQty:     filledQty,
		Status:        mapOrderStatus(r.Status),
		CreatedAt:     time.UnixMilli(createdAt),
		UpdatedAt:     time.UnixMilli(updatedAt),
	}
}

func mapOrderStatus(s string) exchange.OrderStatus {
	switch s {
	case "NEW":
		return exchange.OrderStatusNew
	case "PARTIALLY_FILLED":
		return exchange.OrderStatusPartiallyFilled
	case "FILLED":
		return exchange.OrderStatusFilled
	case "CANCELED", "CANCELLED":
		return exchange.OrderStatusCanceled
	default:
		return exchange.OrderStatus(s)
	}
}

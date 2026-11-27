package ordersapi

import "time"

const (
	OrderPlacedName    = "orders.order_placed.v1"
	OrderCancelledName = "orders.order_cancelled.v1"
)

type OrderPlaced struct {
	OrderID     string    `json:"order_id"`
	CustomerID  string    `json:"customer_id"`
	TotalAmount int64     `json:"total_amount"` // minor units
	Currency    string    `json:"currency"`
	PlacedAt    time.Time `json:"placed_at"`
}

func (OrderPlaced) EventName() string { return OrderPlacedName }

type OrderCancelled struct {
	OrderID     string    `json:"order_id"`
	CustomerID  string    `json:"customer_id"`
	CancelledAt time.Time `json:"cancelled_at"`
}

func (OrderCancelled) EventName() string { return OrderCancelledName }

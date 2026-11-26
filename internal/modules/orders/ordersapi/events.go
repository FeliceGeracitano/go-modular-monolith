// Package ordersapi is the orders module's public contract: the only part
// of the module other modules are allowed to import.
//
// It publishes the integration events other modules may subscribe to. Events
// are versioned facts: once published, a version's meaning and fields never
// change in a breaking way. Evolve by adding optional fields or by
// publishing a new version alongside the old one.
package ordersapi

import "time"

// OrderPlacedName is the routing name of OrderPlaced.
const OrderPlacedName = "orders.order_placed.v1"

// OrderPlaced is published when an order has been accepted.
type OrderPlaced struct {
	OrderID     string    `json:"order_id"`
	CustomerID  string    `json:"customer_id"`
	TotalAmount int64     `json:"total_amount"` // minor units, e.g. cents
	Currency    string    `json:"currency"`
	PlacedAt    time.Time `json:"placed_at"`
}

// EventName implements event.Event.
func (OrderPlaced) EventName() string { return OrderPlacedName }

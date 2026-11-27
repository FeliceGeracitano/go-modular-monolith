// Package event defines what an integration event is.
//
// Integration events are facts a module announces to the rest of the system
// ("an order was placed"). They are part of the publishing module's public
// contract and live in its <module>api package.
package event

// Event is implemented by every integration event.
type Event interface {
	// EventName is the stable, versioned name used for routing and storage,
	// e.g. "orders.order_placed.v1". Changing the payload in a breaking way
	// means publishing a new name, not editing the old one.
	EventName() string
}

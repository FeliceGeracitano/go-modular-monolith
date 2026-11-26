// Package subscribers is the notifications module's inbound event adapter.
// Like an HTTP handler, it translates a transport message (an integration
// event published by another module) into a use-case call.
package subscribers

import (
	"context"

	"github.com/FeliceGeracitano/go-modular-monolith/internal/modules/notifications/internal/app"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/modules/orders/ordersapi"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/eventbus"
)

// Register subscribes the module's use cases to the events they react to.
func Register(sub eventbus.Subscriber, svc *app.Service) {
	sub.Subscribe(ordersapi.OrderPlacedName, "notifications.order_placed",
		eventbus.Typed(func(ctx context.Context, env eventbus.Envelope, evt ordersapi.OrderPlaced) error {
			return svc.NotifyOrderPlaced(ctx, app.OrderPlaced{
				EventID:     env.ID,
				OrderID:     evt.OrderID,
				CustomerID:  evt.CustomerID,
				TotalAmount: evt.TotalAmount,
				Currency:    evt.Currency,
			})
		}))
}

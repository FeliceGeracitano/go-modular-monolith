package app

import (
	"github.com/FeliceGeracitano/go-modular-monolith/modules/customers"
	"github.com/FeliceGeracitano/go-modular-monolith/modules/notifications"
	"github.com/FeliceGeracitano/go-modular-monolith/modules/orders"
	"github.com/FeliceGeracitano/go-modular-monolith/pkg/module"
)

func (d *deps) modules() []module.Module {
	customersModule := customers.New(customers.Deps{
		Clock: d.clock,
	})

	ordersModule := orders.New(orders.Deps{
		Clock:     d.clock,
		Events:    d.events,
		Customers: customersModule.API(),
	})

	notificationsModule := notifications.New(notifications.Deps{
		Clock:     d.clock,
		Logger:    d.logger,
		Customers: customersModule.API(),
	})

	return []module.Module{
		customersModule,
		ordersModule,
		notificationsModule,
	}
}

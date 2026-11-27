package app

import (
	"github.com/FeliceGeracitano/go-modular-monolith/modules/cart"
	"github.com/FeliceGeracitano/go-modular-monolith/modules/customers"
	"github.com/FeliceGeracitano/go-modular-monolith/modules/notifications"
	"github.com/FeliceGeracitano/go-modular-monolith/modules/orders"
	"github.com/FeliceGeracitano/go-modular-monolith/modules/products"
	"github.com/FeliceGeracitano/go-modular-monolith/pkg/module"
)

func (d *deps) modules() []module.Module {
	customersModule := customers.New(customers.Deps{
		Clock: d.clock,
	})

	productsModule := products.New(products.Deps{
		Clock: d.clock,
	})

	cartModule := cart.New(cart.Deps{
		Customers: customersModule.API(),
		Products:  productsModule.API(),
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
		productsModule,
		cartModule,
		ordersModule,
		notificationsModule,
	}
}

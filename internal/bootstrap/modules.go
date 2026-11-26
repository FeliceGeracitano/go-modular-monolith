package bootstrap

import (
	"github.com/FeliceGeracitano/go-modular-monolith/internal/modules/customers"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/modules/notifications"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/modules/orders"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/module"
)

// modules constructs every business module. This function is the system's
// dependency graph: a module that needs another one receives that module's
// public API here, explicitly, and nowhere else.
//
// `make module name=<name>` adds new modules at the marker below.
func (p *platform) modules() ([]module.Module, error) {
	var modules []module.Module

	customersDB, err := p.moduleDB("customers")
	if err != nil {
		return nil, err
	}
	customersModule := customers.New(customers.Deps{
		DB:    customersDB,
		Clock: p.clock,
	})
	modules = append(modules, customersModule)

	ordersDB, err := p.moduleDB("orders")
	if err != nil {
		return nil, err
	}
	ordersModule := orders.New(orders.Deps{
		DB:        ordersDB,
		Clock:     p.clock,
		Events:    p.events,
		Customers: customersModule.API(),
	})
	modules = append(modules, ordersModule)

	notificationsDB, err := p.moduleDB("notifications")
	if err != nil {
		return nil, err
	}
	notificationsModule := notifications.New(notifications.Deps{
		DB:        notificationsDB,
		Clock:     p.clock,
		Logger:    p.logger,
		Customers: customersModule.API(),
	})
	modules = append(modules, notificationsModule)

	// modulegen:wire
	return modules, nil
}

// Package orders is the orders module: placing orders and reading them back.
//
// This package is the module's composition point. Only the composition root
// (internal/bootstrap) may import it. Other modules depend on the contract
// in ordersapi; everything under internal/ is private to the module and the
// Go compiler enforces it.
package orders

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/FeliceGeracitano/go-modular-monolith/internal/kernel/clock"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/modules/customers/customersapi"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/modules/orders/internal/adapters/customersacl"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/modules/orders/internal/adapters/httpapi"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/modules/orders/internal/adapters/pgstore"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/modules/orders/internal/app"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/eventbus"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/module"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/postgres"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Deps are the module's dependencies, supplied by the composition root.
type Deps struct {
	DB    *postgres.DB
	Clock clock.Clock
	// Events stores integration events atomically with the current
	// transaction (the transactional outbox).
	Events app.EventPublisher
	// Customers is the public API of the customers module.
	Customers customersapi.API
}

// Module is the orders module.
type Module struct {
	handler *httpapi.Handler
}

var _ module.Module = (*Module)(nil)

// New assembles the module.
func New(deps Deps) *Module {
	svc := app.NewService(
		pgstore.NewRepository(deps.DB),
		customersacl.NewDirectory(deps.Customers),
		deps.Events,
		deps.DB,
		deps.Clock,
	)
	return &Module{handler: httpapi.NewHandler(svc)}
}

// Name implements module.Module.
func (m *Module) Name() string { return "orders" }

// Migrations implements module.Module.
func (m *Module) Migrations() fs.FS {
	sub, err := fs.Sub(migrations, "migrations")
	if err != nil {
		panic(err) // unreachable: the directory is embedded at build time
	}
	return sub
}

// RegisterRoutes implements module.Module.
func (m *Module) RegisterRoutes(mux *http.ServeMux) { m.handler.Register(mux) }

// RegisterSubscribers implements module.Module. Orders consumes no events.
func (m *Module) RegisterSubscribers(eventbus.Subscriber) {}

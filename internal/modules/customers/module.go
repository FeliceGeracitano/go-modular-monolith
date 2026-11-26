// Package customers is the customers module: registering customers and
// answering questions about them.
//
// This package is the module's composition point. Only the composition root
// (internal/bootstrap) may import it. Other modules depend on the contract
// in customersapi; everything under internal/ is private to the module and
// the Go compiler enforces it.
package customers

import (
	"context"
	"embed"
	"io/fs"
	"net/http"

	"github.com/FeliceGeracitano/go-modular-monolith/internal/kernel/clock"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/modules/customers/customersapi"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/modules/customers/internal/adapters/httpapi"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/modules/customers/internal/adapters/pgstore"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/modules/customers/internal/app"
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
}

// Module is the customers module.
type Module struct {
	svc     *app.Service
	handler *httpapi.Handler
}

var _ module.Module = (*Module)(nil)

// New assembles the module.
func New(deps Deps) *Module {
	svc := app.NewService(pgstore.NewRepository(deps.DB), deps.Clock)
	return &Module{svc: svc, handler: httpapi.NewHandler(svc)}
}

// Name implements module.Module.
func (m *Module) Name() string { return "customers" }

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

// RegisterSubscribers implements module.Module. Customers consumes no events.
func (m *Module) RegisterSubscribers(eventbus.Subscriber) {}

// API returns the module's public, in-process API for other modules.
func (m *Module) API() customersapi.API { return api{svc: m.svc} }

// api adapts the internal service to the public contract, translating the
// domain model into the contract's DTOs.
type api struct {
	svc *app.Service
}

func (a api) GetCustomer(ctx context.Context, id string) (customersapi.Customer, error) {
	c, err := a.svc.Get(ctx, id)
	if err != nil {
		return customersapi.Customer{}, err
	}
	return customersapi.Customer{ID: c.ID, Name: c.Name, Email: c.Email}, nil
}

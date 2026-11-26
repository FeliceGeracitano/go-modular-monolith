// Package notifications is the notifications module: it reacts to things
// that happen elsewhere in the system and tells customers about them.
//
// It has no HTTP API and publishes no contract; it is driven entirely by
// integration events.
package notifications

import (
	"embed"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/FeliceGeracitano/go-modular-monolith/internal/kernel/clock"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/modules/customers/customersapi"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/modules/notifications/internal/adapters/customersacl"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/modules/notifications/internal/adapters/logsender"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/modules/notifications/internal/adapters/pgstore"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/modules/notifications/internal/adapters/subscribers"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/modules/notifications/internal/app"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/eventbus"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/module"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/postgres"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Deps are the module's dependencies, supplied by the composition root.
type Deps struct {
	DB     *postgres.DB
	Clock  clock.Clock
	Logger *slog.Logger
	// Customers is the public API of the customers module.
	Customers customersapi.API
}

// Module is the notifications module.
type Module struct {
	svc *app.Service
}

var _ module.Module = (*Module)(nil)

// New assembles the module.
func New(deps Deps) *Module {
	svc := app.NewService(
		pgstore.NewRepository(deps.DB),
		customersacl.NewDirectory(deps.Customers),
		logsender.New(deps.Logger),
		deps.DB,
		deps.Clock,
	)
	return &Module{svc: svc}
}

// Name implements module.Module.
func (m *Module) Name() string { return "notifications" }

// Migrations implements module.Module.
func (m *Module) Migrations() fs.FS {
	sub, err := fs.Sub(migrations, "migrations")
	if err != nil {
		panic(err) // unreachable: the directory is embedded at build time
	}
	return sub
}

// RegisterRoutes implements module.Module. Notifications has no HTTP API.
func (m *Module) RegisterRoutes(*http.ServeMux) {}

// RegisterSubscribers implements module.Module.
func (m *Module) RegisterSubscribers(sub eventbus.Subscriber) {
	subscribers.Register(sub, m.svc)
}

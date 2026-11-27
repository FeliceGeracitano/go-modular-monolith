package notifications

import (
	"log/slog"
	"net/http"

	"github.com/FeliceGeracitano/go-modular-monolith/modules/customers/customersapi"
	core "github.com/FeliceGeracitano/go-modular-monolith/modules/notifications/internal"
	"github.com/FeliceGeracitano/go-modular-monolith/pkg/clock"
	"github.com/FeliceGeracitano/go-modular-monolith/pkg/eventbus"
	"github.com/FeliceGeracitano/go-modular-monolith/pkg/module"
)

type Deps struct {
	Clock     clock.Clock
	Logger    *slog.Logger
	Customers customersapi.API
}

type Module struct {
	svc     *core.Service
	handler *core.HTTPHandler
}

var _ module.Module = (*Module)(nil)

func New(deps Deps) *Module {
	svc := core.NewService(
		core.NewMemoryStore(),
		core.NewCustomerDirectory(deps.Customers),
		core.NewLogSender(deps.Logger),
		deps.Clock,
	)
	return &Module{svc: svc, handler: core.NewHTTPHandler(svc)}
}

func (m *Module) Name() string                            { return "notifications" }
func (m *Module) RegisterRoutes(mux *http.ServeMux)       { m.handler.Register(mux) }
func (m *Module) RegisterSubscribers(s eventbus.Subscriber) { core.RegisterSubscribers(s, m.svc) }

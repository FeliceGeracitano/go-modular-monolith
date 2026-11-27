package cart

import (
	"net/http"

	core "github.com/FeliceGeracitano/go-modular-monolith/modules/cart/internal"
	"github.com/FeliceGeracitano/go-modular-monolith/modules/customers/customersapi"
	"github.com/FeliceGeracitano/go-modular-monolith/modules/products/productsapi"
	"github.com/FeliceGeracitano/go-modular-monolith/pkg/eventbus"
	"github.com/FeliceGeracitano/go-modular-monolith/pkg/module"
)

type Deps struct {
	Customers customersapi.API
	Products  productsapi.API
}

type Module struct {
	svc     *core.Service
	handler *core.HTTPHandler
}

var _ module.Module = (*Module)(nil)

func New(deps Deps) *Module {
	svc := core.NewService(
		core.NewMemoryStore(),
		core.NewCustomersACL(deps.Customers),
		core.NewProductsACL(deps.Products),
	)
	return &Module{svc: svc, handler: core.NewHTTPHandler(svc)}
}

func (m *Module) Name() string                            { return "cart" }
func (m *Module) RegisterRoutes(mux *http.ServeMux)       { m.handler.Register(mux) }
func (m *Module) RegisterSubscribers(s eventbus.Subscriber) { core.RegisterSubscribers(s, m.svc) }

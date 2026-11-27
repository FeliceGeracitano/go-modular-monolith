package products

import (
	"context"
	"net/http"

	core "github.com/FeliceGeracitano/go-modular-monolith/modules/products/internal"
	"github.com/FeliceGeracitano/go-modular-monolith/modules/products/productsapi"
	"github.com/FeliceGeracitano/go-modular-monolith/pkg/clock"
	"github.com/FeliceGeracitano/go-modular-monolith/pkg/eventbus"
	"github.com/FeliceGeracitano/go-modular-monolith/pkg/module"
)

type Deps struct {
	Clock clock.Clock
}

type Module struct {
	svc     *core.Service
	handler *core.HTTPHandler
}

var _ module.Module = (*Module)(nil)

func New(deps Deps) *Module {
	svc := core.NewService(core.NewMemoryStore(), deps.Clock)
	return &Module{svc: svc, handler: core.NewHTTPHandler(svc)}
}

func (m *Module) Name() string                            { return "products" }
func (m *Module) RegisterRoutes(mux *http.ServeMux)       { m.handler.Register(mux) }
func (m *Module) RegisterSubscribers(eventbus.Subscriber) {}
func (m *Module) API() productsapi.API                    { return api{svc: m.svc} }

type api struct{ svc *core.Service }

func (a api) GetBySKU(ctx context.Context, sku string) (productsapi.Product, error) {
	p, err := a.svc.Get(ctx, sku)
	if err != nil {
		return productsapi.Product{}, err
	}
	return productsapi.Product{
		SKU:      p.SKU,
		Name:     p.Name,
		Price:    p.Price,
		Currency: p.Currency,
		Stock:    p.Stock,
	}, nil
}

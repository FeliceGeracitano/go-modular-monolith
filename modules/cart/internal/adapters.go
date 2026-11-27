package core

import (
	"context"
	"net/http"

	"github.com/FeliceGeracitano/go-modular-monolith/modules/customers/customersapi"
	"github.com/FeliceGeracitano/go-modular-monolith/modules/orders/ordersapi"
	"github.com/FeliceGeracitano/go-modular-monolith/modules/products/productsapi"
	"github.com/FeliceGeracitano/go-modular-monolith/pkg/apperr"
	"github.com/FeliceGeracitano/go-modular-monolith/pkg/eventbus"
	"github.com/FeliceGeracitano/go-modular-monolith/pkg/httpx"
)

type CustomersACL struct{ customers customersapi.API }

func NewCustomersACL(customers customersapi.API) *CustomersACL {
	return &CustomersACL{customers: customers}
}

func (a *CustomersACL) Exists(ctx context.Context, customerID string) (bool, error) {
	_, err := a.customers.GetCustomer(ctx, customerID)
	if apperr.KindOf(err) == apperr.KindNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

type ProductsACL struct{ products productsapi.API }

func NewProductsACL(products productsapi.API) *ProductsACL {
	return &ProductsACL{products: products}
}

func (a *ProductsACL) GetBySKU(ctx context.Context, sku string) (ProductInfo, bool, error) {
	p, err := a.products.GetBySKU(ctx, sku)
	if apperr.KindOf(err) == apperr.KindNotFound {
		return ProductInfo{}, false, nil
	}
	if err != nil {
		return ProductInfo{}, false, err
	}
	return ProductInfo{
		SKU:      p.SKU,
		Name:     p.Name,
		Price:    p.Price,
		Currency: p.Currency,
		Stock:    p.Stock,
	}, true, nil
}

func RegisterSubscribers(sub eventbus.Subscriber, svc *Service) {
	sub.Subscribe(ordersapi.OrderPlacedName, "cart.clear_on_order_placed",
		eventbus.Typed(func(ctx context.Context, _ eventbus.Envelope, evt ordersapi.OrderPlaced) error {
			return svc.Clear(ctx, evt.CustomerID)
		}))
}

type HTTPHandler struct{ svc *Service }

func NewHTTPHandler(svc *Service) *HTTPHandler {
	return &HTTPHandler{svc: svc}
}

func (h *HTTPHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/carts/{customer_id}", h.get)
	mux.HandleFunc("POST /api/v1/carts/{customer_id}/items", h.addItem)
	mux.HandleFunc("DELETE /api/v1/carts/{customer_id}/items/{sku}", h.removeItem)
	mux.HandleFunc("DELETE /api/v1/carts/{customer_id}", h.clear)
}

type addItemRequest struct {
	SKU      string `json:"sku"`
	Quantity int64  `json:"quantity"`
}

type itemResponse struct {
	SKU       string `json:"sku"`
	Name      string `json:"name"`
	Quantity  int64  `json:"quantity"`
	UnitPrice int64  `json:"unit_price"`
	Currency  string `json:"currency"`
}

type cartResponse struct {
	CustomerID  string         `json:"customer_id"`
	Currency    string         `json:"currency,omitempty"`
	TotalAmount int64          `json:"total_amount"`
	Items       []itemResponse `json:"items"`
}

func toCartResponse(c Cart) cartResponse {
	items := make([]itemResponse, 0, len(c.Items))
	for _, it := range c.Items {
		items = append(items, itemResponse{
			SKU:       it.SKU,
			Name:      it.Name,
			Quantity:  it.Quantity,
			UnitPrice: it.UnitPrice,
			Currency:  it.Currency,
		})
	}
	return cartResponse{
		CustomerID:  c.CustomerID,
		Currency:    c.Currency,
		TotalAmount: c.TotalAmount(),
		Items:       items,
	}
}

func (h *HTTPHandler) get(w http.ResponseWriter, r *http.Request) {
	c, err := h.svc.Get(r.Context(), r.PathValue("customer_id"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toCartResponse(c))
}

func (h *HTTPHandler) addItem(w http.ResponseWriter, r *http.Request) {
	var req addItemRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	c, err := h.svc.AddItem(r.Context(), r.PathValue("customer_id"), req.SKU, req.Quantity)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toCartResponse(c))
}

func (h *HTTPHandler) removeItem(w http.ResponseWriter, r *http.Request) {
	c, err := h.svc.RemoveItem(r.Context(), r.PathValue("customer_id"), r.PathValue("sku"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toCartResponse(c))
}

func (h *HTTPHandler) clear(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Clear(r.Context(), r.PathValue("customer_id")); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

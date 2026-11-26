// Package httpapi is the HTTP adapter for the orders module. It translates
// HTTP to use-case calls and back; it holds no business logic.
package httpapi

import (
	"net/http"
	"time"

	"github.com/FeliceGeracitano/go-modular-monolith/internal/modules/orders/internal/app"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/modules/orders/internal/domain"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/httpx"
)

// Handler serves the orders endpoints.
type Handler struct {
	svc *app.Service
}

// NewHandler returns a Handler.
func NewHandler(svc *app.Service) *Handler {
	return &Handler{svc: svc}
}

// Register mounts the module's routes.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/orders", h.place)
	mux.HandleFunc("GET /api/v1/orders/{id}", h.get)
}

type lineRequest struct {
	SKU       string `json:"sku"`
	Quantity  int64  `json:"quantity"`
	UnitPrice int64  `json:"unit_price"` // minor units, e.g. cents
}

type placeRequest struct {
	CustomerID string        `json:"customer_id"`
	Currency   string        `json:"currency"`
	Lines      []lineRequest `json:"lines"`
}

type lineResponse struct {
	SKU       string `json:"sku"`
	Quantity  int64  `json:"quantity"`
	UnitPrice int64  `json:"unit_price"`
}

type orderResponse struct {
	ID          string         `json:"id"`
	CustomerID  string         `json:"customer_id"`
	Status      string         `json:"status"`
	Currency    string         `json:"currency"`
	TotalAmount int64          `json:"total_amount"`
	Lines       []lineResponse `json:"lines"`
	PlacedAt    time.Time      `json:"placed_at"`
}

func toResponse(o domain.Order) orderResponse {
	total := o.Total()
	lines := make([]lineResponse, 0, len(o.Lines))
	for _, l := range o.Lines {
		lines = append(lines, lineResponse{SKU: l.SKU, Quantity: l.Quantity, UnitPrice: l.UnitPrice.Amount})
	}
	return orderResponse{
		ID:          o.ID,
		CustomerID:  o.CustomerID,
		Status:      string(o.Status),
		Currency:    total.Currency,
		TotalAmount: total.Amount,
		Lines:       lines,
		PlacedAt:    o.PlacedAt,
	}
}

func (h *Handler) place(w http.ResponseWriter, r *http.Request) {
	var req placeRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}

	cmd := app.PlaceOrder{CustomerID: req.CustomerID, Currency: req.Currency}
	for _, l := range req.Lines {
		cmd.Lines = append(cmd.Lines, app.PlaceOrderLine{SKU: l.SKU, Quantity: l.Quantity, UnitPrice: l.UnitPrice})
	}

	order, err := h.svc.Place(r.Context(), cmd)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/orders/"+order.ID)
	httpx.JSON(w, http.StatusCreated, toResponse(order))
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	order, err := h.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toResponse(order))
}

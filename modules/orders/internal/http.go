package core

import (
	"net/http"
	"strconv"
	"time"

	"github.com/FeliceGeracitano/go-modular-monolith/pkg/httpx"
)

type HTTPHandler struct {
	svc *Service
}

func NewHTTPHandler(svc *Service) *HTTPHandler {
	return &HTTPHandler{svc: svc}
}

func (h *HTTPHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/orders", h.place)
	mux.HandleFunc("GET /api/v1/orders", h.list)
	mux.HandleFunc("GET /api/v1/orders/{id}", h.get)
	mux.HandleFunc("POST /api/v1/orders/{id}/cancel", h.cancel)
}

type lineRequest struct {
	SKU       string `json:"sku"`
	Quantity  int64  `json:"quantity"`
	UnitPrice int64  `json:"unit_price"`
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
	CancelledAt *time.Time     `json:"cancelled_at,omitempty"`
}

type listOrdersResponse struct {
	Items  []orderResponse `json:"items"`
	Total  int             `json:"total"`
	Limit  int             `json:"limit"`
	Offset int             `json:"offset"`
}

func toOrderResponse(o Order) orderResponse {
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
		CancelledAt: o.CancelledAt,
	}
}

func (h *HTTPHandler) place(w http.ResponseWriter, r *http.Request) {
	var req placeRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	cmd := PlaceOrder{CustomerID: req.CustomerID, Currency: req.Currency}
	for _, l := range req.Lines {
		cmd.Lines = append(cmd.Lines, PlaceOrderLine{SKU: l.SKU, Quantity: l.Quantity, UnitPrice: l.UnitPrice})
	}
	order, err := h.svc.Place(r.Context(), cmd)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/orders/"+order.ID)
	httpx.JSON(w, http.StatusCreated, toOrderResponse(order))
}

func (h *HTTPHandler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))

	res, err := h.svc.List(r.Context(), ListFilter{
		CustomerID: q.Get("customer_id"),
		Status:     Status(q.Get("status")),
		Limit:      limit,
		Offset:     offset,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	items := make([]orderResponse, 0, len(res.Items))
	for _, o := range res.Items {
		items = append(items, toOrderResponse(o))
	}
	httpx.JSON(w, http.StatusOK, listOrdersResponse{
		Items:  items,
		Total:  res.Total,
		Limit:  res.Limit,
		Offset: res.Offset,
	})
}

func (h *HTTPHandler) get(w http.ResponseWriter, r *http.Request) {
	order, err := h.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toOrderResponse(order))
}

func (h *HTTPHandler) cancel(w http.ResponseWriter, r *http.Request) {
	order, err := h.svc.Cancel(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toOrderResponse(order))
}

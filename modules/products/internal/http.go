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
	mux.HandleFunc("POST /api/v1/products", h.create)
	mux.HandleFunc("GET /api/v1/products", h.list)
	mux.HandleFunc("GET /api/v1/products/{sku}", h.get)
	mux.HandleFunc("PATCH /api/v1/products/{sku}", h.update)
	mux.HandleFunc("DELETE /api/v1/products/{sku}", h.delete)
}

type createRequest struct {
	SKU      string `json:"sku"`
	Name     string `json:"name"`
	Price    int64  `json:"price"`
	Currency string `json:"currency"`
	Stock    int64  `json:"stock"`
}

type updateRequest struct {
	Name  *string `json:"name"`
	Price *int64  `json:"price"`
	Stock *int64  `json:"stock"`
}

type productResponse struct {
	SKU       string    `json:"sku"`
	Name      string    `json:"name"`
	Price     int64     `json:"price"`
	Currency  string    `json:"currency"`
	Stock     int64     `json:"stock"`
	CreatedAt time.Time `json:"created_at"`
}

type listProductsResponse struct {
	Items  []productResponse `json:"items"`
	Total  int               `json:"total"`
	Limit  int               `json:"limit"`
	Offset int               `json:"offset"`
}

func toProductResponse(p Product) productResponse {
	return productResponse{
		SKU:       p.SKU,
		Name:      p.Name,
		Price:     p.Price,
		Currency:  p.Currency,
		Stock:     p.Stock,
		CreatedAt: p.CreatedAt,
	}
}

func (h *HTTPHandler) create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	p, err := h.svc.Create(r.Context(), CreateProduct{
		SKU:      req.SKU,
		Name:     req.Name,
		Price:    req.Price,
		Currency: req.Currency,
		Stock:    req.Stock,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/products/"+p.SKU)
	httpx.JSON(w, http.StatusCreated, toProductResponse(p))
}

func (h *HTTPHandler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))

	res, err := h.svc.List(r.Context(), limit, offset)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	items := make([]productResponse, 0, len(res.Items))
	for _, p := range res.Items {
		items = append(items, toProductResponse(p))
	}
	httpx.JSON(w, http.StatusOK, listProductsResponse{
		Items:  items,
		Total:  res.Total,
		Limit:  res.Limit,
		Offset: res.Offset,
	})
}

func (h *HTTPHandler) get(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.Get(r.Context(), r.PathValue("sku"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toProductResponse(p))
}

func (h *HTTPHandler) update(w http.ResponseWriter, r *http.Request) {
	var req updateRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	p, err := h.svc.Update(r.Context(), r.PathValue("sku"), UpdateProduct{
		Name:  req.Name,
		Price: req.Price,
		Stock: req.Stock,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toProductResponse(p))
}

func (h *HTTPHandler) delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), r.PathValue("sku")); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

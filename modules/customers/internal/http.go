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
	mux.HandleFunc("POST /api/v1/customers", h.register)
	mux.HandleFunc("GET /api/v1/customers", h.list)
	mux.HandleFunc("GET /api/v1/customers/{id}", h.get)
	mux.HandleFunc("PATCH /api/v1/customers/{id}", h.update)
	mux.HandleFunc("DELETE /api/v1/customers/{id}", h.delete)
}

type registerRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type updateRequest struct {
	Name  *string `json:"name"`
	Email *string `json:"email"`
}

type customerResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

type listCustomersResponse struct {
	Items  []customerResponse `json:"items"`
	Total  int                `json:"total"`
	Limit  int                `json:"limit"`
	Offset int                `json:"offset"`
}

func toCustomerResponse(c Customer) customerResponse {
	return customerResponse{ID: c.ID, Name: c.Name, Email: c.Email, CreatedAt: c.CreatedAt}
}

func (h *HTTPHandler) register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	customer, err := h.svc.Register(r.Context(), RegisterCustomer{Name: req.Name, Email: req.Email})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/customers/"+customer.ID)
	httpx.JSON(w, http.StatusCreated, toCustomerResponse(customer))
}

func (h *HTTPHandler) list(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	res, err := h.svc.List(r.Context(), limit, offset)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	items := make([]customerResponse, 0, len(res.Items))
	for _, c := range res.Items {
		items = append(items, toCustomerResponse(c))
	}
	httpx.JSON(w, http.StatusOK, listCustomersResponse{
		Items:  items,
		Total:  res.Total,
		Limit:  res.Limit,
		Offset: res.Offset,
	})
}

func (h *HTTPHandler) get(w http.ResponseWriter, r *http.Request) {
	customer, err := h.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toCustomerResponse(customer))
}

func (h *HTTPHandler) update(w http.ResponseWriter, r *http.Request) {
	var req updateRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	customer, err := h.svc.Update(r.Context(), r.PathValue("id"), UpdateCustomer{
		Name:  req.Name,
		Email: req.Email,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toCustomerResponse(customer))
}

func (h *HTTPHandler) delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), r.PathValue("id")); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

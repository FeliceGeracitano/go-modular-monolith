// Package httpapi is the HTTP adapter for the customers module. It
// translates HTTP to use-case calls and back; it holds no business logic.
package httpapi

import (
	"net/http"
	"time"

	"github.com/FeliceGeracitano/go-modular-monolith/internal/modules/customers/internal/app"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/modules/customers/internal/domain"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/httpx"
)

// Handler serves the customers endpoints.
type Handler struct {
	svc *app.Service
}

// NewHandler returns a Handler.
func NewHandler(svc *app.Service) *Handler {
	return &Handler{svc: svc}
}

// Register mounts the module's routes.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/customers", h.register)
	mux.HandleFunc("GET /api/v1/customers/{id}", h.get)
}

// The request and response types below are the HTTP contract. They are
// separate from the domain model on purpose: renaming a domain field must
// never silently change the public API.

type registerRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type customerResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

func toResponse(c domain.Customer) customerResponse {
	return customerResponse{ID: c.ID, Name: c.Name, Email: c.Email, CreatedAt: c.CreatedAt}
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	customer, err := h.svc.Register(r.Context(), app.RegisterCustomer{Name: req.Name, Email: req.Email})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/customers/"+customer.ID)
	httpx.JSON(w, http.StatusCreated, toResponse(customer))
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	customer, err := h.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toResponse(customer))
}

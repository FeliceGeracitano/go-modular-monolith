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
	mux.HandleFunc("GET /api/v1/notifications", h.list)
}

type notificationResponse struct {
	ID         string    `json:"id"`
	EventID    string    `json:"event_id"`
	CustomerID string    `json:"customer_id"`
	Recipient  string    `json:"recipient"`
	Subject    string    `json:"subject"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
}

type listNotificationsResponse struct {
	Items  []notificationResponse `json:"items"`
	Total  int                    `json:"total"`
	Limit  int                    `json:"limit"`
	Offset int                    `json:"offset"`
}

func (h *HTTPHandler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))

	res, err := h.svc.List(r.Context(), q.Get("customer_id"), limit, offset)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	items := make([]notificationResponse, 0, len(res.Items))
	for _, n := range res.Items {
		items = append(items, notificationResponse{
			ID:         n.ID,
			EventID:    n.EventID,
			CustomerID: n.CustomerID,
			Recipient:  n.Recipient,
			Subject:    n.Subject,
			Body:       n.Body,
			CreatedAt:  n.CreatedAt,
		})
	}
	httpx.JSON(w, http.StatusOK, listNotificationsResponse{
		Items:  items,
		Total:  res.Total,
		Limit:  res.Limit,
		Offset: res.Offset,
	})
}

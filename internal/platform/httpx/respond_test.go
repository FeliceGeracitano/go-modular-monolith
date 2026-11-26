package httpx_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/FeliceGeracitano/go-modular-monolith/internal/kernel/apperr"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/httpx"
)

func TestErrorMapsKindsToStatuses(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
		wantDetail string
	}{
		{"invalid", apperr.Invalid("x.invalid", "bad input"), http.StatusUnprocessableEntity, "x.invalid", "bad input"},
		{"not found", apperr.NotFound("x.not_found", "missing"), http.StatusNotFound, "x.not_found", "missing"},
		{"conflict", apperr.Conflict("x.conflict", "taken"), http.StatusConflict, "x.conflict", "taken"},
		{"deadline exceeded", fmt.Errorf("query: %w", context.DeadlineExceeded), http.StatusGatewayTimeout, "timeout", ""},
		{"client went away", fmt.Errorf("query: %w", context.Canceled), http.StatusRequestTimeout, "canceled", ""},
		{"unclassified errors are opaque", errors.New("pq: password authentication failed"), http.StatusInternalServerError, "internal", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			httpx.Error(rec, httptest.NewRequest(http.MethodGet, "/", nil), tt.err)

			var got httpx.Problem
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("response is not JSON: %v", err)
			}
			if rec.Code != tt.wantStatus || got.Status != tt.wantStatus {
				t.Errorf("status = %d (body %d), want %d", rec.Code, got.Status, tt.wantStatus)
			}
			if got.Code != tt.wantCode || got.Detail != tt.wantDetail {
				t.Errorf("code/detail = %q/%q, want %q/%q", got.Code, got.Detail, tt.wantCode, tt.wantDetail)
			}
		})
	}
}

func TestDecodeRejectsBadBodies(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}
	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{"valid", `{"name":"Ada"}`, false},
		{"unknown field", `{"name":"Ada","admin":true}`, true},
		{"trailing data", `{"name":"Ada"}{"name":"Eve"}`, true},
		{"not json", `name=Ada`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var dst payload
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			err := httpx.Decode(httptest.NewRecorder(), req, &dst)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Decode() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && !errors.Is(err, httpx.ErrMalformedBody) {
				t.Fatalf("error = %v, want ErrMalformedBody", err)
			}
		})
	}
}

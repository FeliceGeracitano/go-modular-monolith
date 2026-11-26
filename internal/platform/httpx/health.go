package httpx

import (
	"context"
	"net/http"
	"time"
)

// Check reports whether a dependency is usable.
type Check func(ctx context.Context) error

// RegisterHealth mounts the probe endpoints:
//
//   - GET /healthz: liveness. The process is up; never checks dependencies,
//     so a database outage does not get the process restarted.
//   - GET /readyz: readiness. Every check passes; failing takes the instance
//     out of rotation without killing it.
func RegisterHealth(mux *http.ServeMux, checks map[string]Check) {
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		status := http.StatusOK
		results := make(map[string]string, len(checks))
		for name, check := range checks {
			if err := check(ctx); err != nil {
				status = http.StatusServiceUnavailable
				results[name] = "unavailable"
				continue
			}
			results[name] = "ok"
		}
		JSON(w, status, map[string]any{"checks": results})
	})
}

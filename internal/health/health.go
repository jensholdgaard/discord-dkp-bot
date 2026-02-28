package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/jensholdgaard/discord-dkp-bot/internal/clock"
)

// Status represents a health check result.
type Status struct {
	Status    string            `json:"status"`
	Checks    map[string]string `json:"checks,omitempty"`
	Timestamp string            `json:"timestamp"`
}

// Checker defines a named health check function.
type Checker struct {
	Name  string
	Check func(ctx context.Context) error
}

// Handler provides HTTP health check endpoints.
type Handler struct {
	mu       sync.RWMutex
	ready    bool
	checkers []Checker
	clock    clock.Clock
}

// NewHandler creates a new health handler with the given checkers.
func NewHandler(clk clock.Clock, checkers ...Checker) *Handler {
	return &Handler{checkers: checkers, clock: clk}
}

// SetReady marks the service as ready to receive traffic.
func (h *Handler) SetReady(ready bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.ready = ready
}

// LivenessHandler returns HTTP 200 if the process is alive.
func (h *Handler) LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, Status{
			Status:    "ok",
			Timestamp: h.clock.Now().UTC().Format(time.RFC3339),
		})
	}
}

// ReadinessHandler returns HTTP 200 if the service is ready.
func (h *Handler) ReadinessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h.mu.RLock()
		ready := h.ready
		h.mu.RUnlock()

		if !ready {
			writeJSON(w, http.StatusServiceUnavailable, Status{
				Status:    "not_ready",
				Timestamp: h.clock.Now().UTC().Format(time.RFC3339),
			})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		checks := make(map[string]string)
		allOK := true
		for _, c := range h.checkers {
			if err := c.Check(ctx); err != nil {
				checks[c.Name] = err.Error()
				allOK = false
			} else {
				checks[c.Name] = "ok"
			}
		}

		status := "ready"
		code := http.StatusOK
		if !allOK {
			status = "not_ready"
			code = http.StatusServiceUnavailable
		}

		writeJSON(w, code, Status{
			Status:    status,
			Checks:    checks,
			Timestamp: h.clock.Now().UTC().Format(time.RFC3339),
		})
	}
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

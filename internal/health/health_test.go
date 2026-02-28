package health_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jensholdgaard/discord-dkp-bot/internal/clock"
	"github.com/jensholdgaard/discord-dkp-bot/internal/health"
)

var testClk = clock.Real{}

func TestLivenessHandler(t *testing.T) {
	h := health.NewHandler(testClk)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	h.LivenessHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusOK)
	}
	var s health.Status
	if err := json.NewDecoder(rec.Body).Decode(&s); err != nil {
		t.Fatal(err)
	}
	if s.Status != "ok" {
		t.Errorf("got status %q, want %q", s.Status, "ok")
	}
}

func TestReadinessHandler(t *testing.T) {
	tests := []struct {
		name       string
		ready      bool
		checkers   []health.Checker
		wantCode   int
		wantStatus string
	}{
		{
			name:       "not ready",
			ready:      false,
			wantCode:   http.StatusServiceUnavailable,
			wantStatus: "not_ready",
		},
		{
			name:       "ready no checkers",
			ready:      true,
			wantCode:   http.StatusOK,
			wantStatus: "ready",
		},
		{
			name:  "ready all checks pass",
			ready: true,
			checkers: []health.Checker{
				{Name: "db", Check: func(ctx context.Context) error { return nil }},
			},
			wantCode:   http.StatusOK,
			wantStatus: "ready",
		},
		{
			name:  "ready but check fails",
			ready: true,
			checkers: []health.Checker{
				{Name: "db", Check: func(ctx context.Context) error { return errors.New("connection refused") }},
			},
			wantCode:   http.StatusServiceUnavailable,
			wantStatus: "not_ready",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := health.NewHandler(testClk, tt.checkers...)
			h.SetReady(tt.ready)

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/readyz", nil)

			h.ReadinessHandler().ServeHTTP(rec, req)

			if rec.Code != tt.wantCode {
				t.Errorf("got status %d, want %d", rec.Code, tt.wantCode)
			}
			var s health.Status
			if err := json.NewDecoder(rec.Body).Decode(&s); err != nil {
				t.Fatal(err)
			}
			if s.Status != tt.wantStatus {
				t.Errorf("got status %q, want %q", s.Status, tt.wantStatus)
			}
		})
	}
}

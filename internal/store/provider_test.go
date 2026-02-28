package store_test

import (
	"context"
	"testing"

	"github.com/jensholdgaard/discord-dkp-bot/internal/clock"
	"github.com/jensholdgaard/discord-dkp-bot/internal/config"
	"github.com/jensholdgaard/discord-dkp-bot/internal/store"

	// Import drivers so their init() functions register them.
	_ "github.com/jensholdgaard/discord-dkp-bot/internal/store/entstore"
	_ "github.com/jensholdgaard/discord-dkp-bot/internal/store/postgres"
)

// fakeDriver is a store.Driver that always succeeds without connecting to a DB.
func fakeDriver(_ context.Context, _ config.DatabaseConfig, _ clock.Clock) (*store.Repositories, error) {
	return &store.Repositories{}, nil
}

func TestOpen(t *testing.T) {
	// Register a test driver.
	store.Register("test-driver", fakeDriver)

	tests := []struct {
		name    string
		driver  string
		wantErr bool
	}{
		{
			name:    "registered driver succeeds",
			driver:  "test-driver",
			wantErr: false,
		},
		{
			name:    "unknown driver fails",
			driver:  "nonexistent",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.DatabaseConfig{Driver: tt.driver}
			_, err := store.Open(context.Background(), cfg, clock.Real{})
			if (err != nil) != tt.wantErr {
				t.Errorf("Open(driver=%q) error = %v, wantErr %v", tt.driver, err, tt.wantErr)
			}
		})
	}
}

func TestRegister(t *testing.T) {
	// Registering "sqlx" and "ent" should already be done via init() imports.
	// This test verifies that re-registration does not panic and that the
	// drivers are actually registered by checking Open does not return
	// "unknown driver" for them.
	//
	// Note: these drivers will fail to actually connect (no DB), so we only
	// check that the error is NOT "unknown store driver".

	for _, driver := range []string{"sqlx", "ent"} {
		t.Run(driver, func(t *testing.T) {
			cfg := config.DatabaseConfig{Driver: driver, Host: "localhost", Port: 5432}
			_, err := store.Open(context.Background(), cfg, clock.Real{})
			if err == nil {
				t.Fatal("expected error (no DB running), got nil")
			}
			// The error should be a connection error, not an unknown-driver error.
			want := "unknown store driver"
			if containsStr(err.Error(), want) {
				t.Errorf("expected connection error, got unknown driver error: %v", err)
			}
		})
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && searchStr(s, substr)
}

func searchStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

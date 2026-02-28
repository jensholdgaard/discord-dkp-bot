package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jensholdgaard/discord-dkp-bot/internal/config"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
		check   func(t *testing.T, cfg *config.Config)
	}{
		{
			name: "valid full config",
			yaml: `
discord:
  token: "test-token"
  guild_id: "123456"
database:
  host: "db.example.com"
  port: 5433
  user: "dkpbot"
  password: "secret"
  dbname: "dkp"
  sslmode: "require"
  driver: "sqlx"
server:
  port: 9090
telemetry:
  service_name: "my-bot"
  otlp_endpoint: "localhost:4318"
`,
			wantErr: false,
			check: func(t *testing.T, cfg *config.Config) {
				t.Helper()
				if cfg.Discord.Token != "test-token" {
					t.Errorf("got token %q, want %q", cfg.Discord.Token, "test-token")
				}
				if cfg.Database.Port != 5433 {
					t.Errorf("got db port %d, want %d", cfg.Database.Port, 5433)
				}
				if cfg.Server.Port != 9090 {
					t.Errorf("got server port %d, want %d", cfg.Server.Port, 9090)
				}
				if cfg.Telemetry.ServiceName != "my-bot" {
					t.Errorf("got service name %q, want %q", cfg.Telemetry.ServiceName, "my-bot")
				}
			},
		},
		{
			name: "defaults applied",
			yaml: `
discord:
  token: "tok"
`,
			wantErr: false,
			check: func(t *testing.T, cfg *config.Config) {
				t.Helper()
				if cfg.Database.Host != "localhost" {
					t.Errorf("got db host %q, want %q", cfg.Database.Host, "localhost")
				}
				if cfg.Database.Port != 5432 {
					t.Errorf("got db port %d, want %d", cfg.Database.Port, 5432)
				}
				if cfg.Server.Port != 8080 {
					t.Errorf("got server port %d, want %d", cfg.Server.Port, 8080)
				}
				if cfg.Telemetry.ServiceName != "dkpbot" {
					t.Errorf("got service name %q, want %q", cfg.Telemetry.ServiceName, "dkpbot")
				}
			},
		},
		{
			name:    "invalid yaml",
			yaml:    `{{{invalid`,
			wantErr: true,
		},
		{
			name: "ent driver accepted",
			yaml: `
discord:
  token: "tok"
database:
  driver: "ent"
`,
			wantErr: false,
			check: func(t *testing.T, cfg *config.Config) {
				t.Helper()
				if cfg.Database.Driver != "ent" {
					t.Errorf("got driver %q, want %q", cfg.Database.Driver, "ent")
				}
			},
		},
		{
			name: "invalid driver rejected",
			yaml: `
discord:
  token: "tok"
database:
  driver: "mongodb"
`,
			wantErr: true,
		},
		{
			name: "default driver is sqlx",
			yaml: `
discord:
  token: "tok"
`,
			wantErr: false,
			check: func(t *testing.T, cfg *config.Config) {
				t.Helper()
				if cfg.Database.Driver != "sqlx" {
					t.Errorf("got driver %q, want %q", cfg.Database.Driver, "sqlx")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")
			if err := os.WriteFile(path, []byte(tt.yaml), 0o644); err != nil {
				t.Fatal(err)
			}

			cfg, err := config.Load(path)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Load() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.check != nil && cfg != nil {
				tt.check(t, cfg)
			}
		})
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := config.Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestDatabaseConfig_DSN(t *testing.T) {
	cfg := config.DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "user",
		Password: "pass",
		DBName:   "testdb",
		SSLMode:  "disable",
	}
	want := "host=localhost port=5432 user=user password=pass dbname=testdb sslmode=disable"
	if got := cfg.DSN(); got != want {
		t.Errorf("DSN() = %q, want %q", got, want)
	}
}

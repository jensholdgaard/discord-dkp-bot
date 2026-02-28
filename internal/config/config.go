package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration.
type Config struct {
	Discord        DiscordConfig        `yaml:"discord"`
	Database       DatabaseConfig       `yaml:"database"`
	Server         ServerConfig         `yaml:"server"`
	Telemetry      TelemetryConfig      `yaml:"telemetry"`
	LeaderElection LeaderElectionConfig `yaml:"leader_election"`
}

// DiscordConfig holds Discord bot settings.
type DiscordConfig struct {
	Token   string `yaml:"token"`
	GuildID string `yaml:"guild_id"`
}

// DatabaseConfig holds database connection settings.
type DatabaseConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	DBName   string `yaml:"dbname"`
	SSLMode  string `yaml:"sslmode"`
	Driver   string `yaml:"driver"` // "sqlx" or "ent"
}

// DSN returns the Postgres connection string.
func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.DBName, d.SSLMode,
	)
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port            int           `yaml:"port"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
}

// TelemetryConfig holds OpenTelemetry settings.
type TelemetryConfig struct {
	ServiceName    string `yaml:"service_name"`
	ServiceVersion string `yaml:"service_version"`
	OTLPEndpoint   string `yaml:"otlp_endpoint"`
	Insecure       bool   `yaml:"insecure"`
}

// LeaderElectionConfig holds Kubernetes leader election settings.
type LeaderElectionConfig struct {
	Enabled        bool          `yaml:"enabled"`
	LeaseName      string        `yaml:"lease_name"`
	LeaseNamespace string        `yaml:"lease_namespace"`
	LeaseDuration  time.Duration `yaml:"lease_duration"`
	RenewDeadline  time.Duration `yaml:"renew_deadline"`
	RetryPeriod    time.Duration `yaml:"retry_period"`
}

// Load reads a YAML configuration file from the given path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	cfg := &Config{
		Server: ServerConfig{
			Port:            8080,
			ShutdownTimeout: 15 * time.Second,
		},
		Database: DatabaseConfig{
			Host:    "localhost",
			Port:    5432,
			SSLMode: "disable",
			Driver:  "sqlx",
		},
		Telemetry: TelemetryConfig{
			ServiceName:    "dkpbot",
			ServiceVersion: "0.1.0",
		},
		LeaderElection: LeaderElectionConfig{
			Enabled:        false,
			LeaseName:      "dkpbot-leader",
			LeaseNamespace: "default",
			LeaseDuration:  15 * time.Second,
			RenewDeadline:  10 * time.Second,
			RetryPeriod:    2 * time.Second,
		},
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return cfg, nil
}

// validate checks configuration invariants.
func (c *Config) validate() error {
	switch c.Database.Driver {
	case "sqlx", "ent":
		// valid
	default:
		return fmt.Errorf("unsupported database driver %q: must be \"sqlx\" or \"ent\"", c.Database.Driver)
	}
	return nil
}

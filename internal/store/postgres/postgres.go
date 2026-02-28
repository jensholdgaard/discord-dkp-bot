package postgres

import (
	"context"
	"fmt"

	"github.com/XSAM/otelsql"
	"github.com/jensholdgaard/discord-dkp-bot/internal/config"
	"github.com/jmoiron/sqlx"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Connect opens and verifies a Postgres connection with OTEL instrumentation.
func Connect(ctx context.Context, cfg config.DatabaseConfig) (*sqlx.DB, error) {
	dsn := cfg.DSN()

	// Register the OTel-instrumented driver wrapping lib/pq.
	driverName, err := otelsql.Register("postgres",
		otelsql.WithAttributes(semconv.DBSystemPostgreSQL),
	)
	if err != nil {
		return nil, fmt.Errorf("registering otel driver: %w", err)
	}

	db, err := sqlx.ConnectContext(ctx, driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("connecting to database: %w", err)
	}

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	return db, nil
}

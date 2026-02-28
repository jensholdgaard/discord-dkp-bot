// Package entstore provides a store.Driver backed by ent-style plain SQL
// executed through database/sql with OTEL instrumentation via otelsql.
//
// This implementation uses the same Postgres database and schema as the sqlx
// driver but accesses it through the standard database/sql interface, which is
// the approach ent uses under the hood. When full ent schema codegen is added
// the raw queries below can be replaced by ent client calls.
package entstore

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/XSAM/otelsql"
	_ "github.com/lib/pq" // postgres driver
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"github.com/jensholdgaard/discord-dkp-bot/internal/clock"
	"github.com/jensholdgaard/discord-dkp-bot/internal/config"
	"github.com/jensholdgaard/discord-dkp-bot/internal/store"
)

// closerFunc adapts a func() error into an io.Closer.
type closerFunc func() error

func (f closerFunc) Close() error { return f() }

func init() {
	store.Register("ent", openEnt)
}

// openEnt is the store.Driver for the "ent" backend.
func openEnt(ctx context.Context, cfg config.DatabaseConfig, clk clock.Clock) (*store.Repositories, error) {
	db, err := Connect(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &store.Repositories{
		Players:  NewPlayerRepo(db, clk),
		Auctions: NewAuctionRepo(db, clk),
		Events:   NewEventStore(db),
		Closer:   closerFunc(db.Close),
		Ping:     db.PingContext,
	}, nil
}

// Connect opens and verifies a Postgres connection via database/sql with OTEL
// instrumentation. This is the connection style ent uses internally.
func Connect(ctx context.Context, cfg config.DatabaseConfig) (*sql.DB, error) {
	dsn := cfg.DSN()

	db, err := otelsql.Open("postgres", dsn,
		otelsql.WithAttributes(semconv.DBSystemPostgreSQL),
	)
	if err != nil {
		return nil, fmt.Errorf("opening ent database: %w", err)
	}

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("pinging ent database: %w", err)
	}

	return db, nil
}

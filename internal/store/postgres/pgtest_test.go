package postgres_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// newTestDB starts a Postgres container, applies the migration, and returns
// a connected *sqlx.DB. The container is automatically terminated when the
// test ends.
func newTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()

	// Locate migration file relative to this source file.
	_, thisFile, _, _ := runtime.Caller(0)
	migrationDir := filepath.Join(filepath.Dir(thisFile), "migrations")

	migrationSQL, err := os.ReadFile(filepath.Join(migrationDir, "001_initial.sql"))
	if err != nil {
		t.Fatalf("reading migration: %v", err)
	}

	ctr, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("dkpbot_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		tcpostgres.WithInitScripts(), // no bundled init scripts
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		t.Fatalf("starting postgres container: %v", err)
	}

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("getting connection string: %v", err)
	}

	db, err := sqlx.Connect("postgres", connStr)
	if err != nil {
		t.Fatalf("connecting to test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Apply migration.
	if _, err := db.ExecContext(ctx, string(migrationSQL)); err != nil {
		t.Fatalf("applying migration: %v", err)
	}

	return db
}

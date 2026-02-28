package postgres

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/jensholdgaard/discord-dkp-bot/internal/event"
)

// EventStore implements event.Store backed by Postgres.
type EventStore struct {
	db *sqlx.DB
}

// NewEventStore returns a new EventStore.
func NewEventStore(db *sqlx.DB) *EventStore {
	return &EventStore{db: db}
}

func (s *EventStore) Append(ctx context.Context, events ...event.Event) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PreparexContext(ctx,
		`INSERT INTO events (aggregate_id, type, data, version) VALUES ($1, $2, $3, $4)`)
	if err != nil {
		return fmt.Errorf("preparing statement: %w", err)
	}
	defer stmt.Close()

	for _, e := range events {
		if _, err := stmt.ExecContext(ctx, e.AggregateID, e.Type, e.Data, e.Version); err != nil {
			return fmt.Errorf("inserting event (aggregate=%s, version=%d): %w", e.AggregateID, e.Version, err)
		}
	}

	return tx.Commit()
}

func (s *EventStore) Load(ctx context.Context, aggregateID string) ([]event.Event, error) {
	var events []event.Event
	err := s.db.SelectContext(ctx, &events,
		`SELECT id, aggregate_id, type, data, version, created_at
		 FROM events WHERE aggregate_id = $1 ORDER BY version ASC`, aggregateID)
	if err != nil {
		return nil, fmt.Errorf("loading events: %w", err)
	}
	return events, nil
}

func (s *EventStore) LoadByType(ctx context.Context, eventType event.Type) ([]event.Event, error) {
	var events []event.Event
	err := s.db.SelectContext(ctx, &events,
		`SELECT id, aggregate_id, type, data, version, created_at
		 FROM events WHERE type = $1 ORDER BY created_at ASC`, eventType)
	if err != nil {
		return nil, fmt.Errorf("loading events by type: %w", err)
	}
	return events, nil
}

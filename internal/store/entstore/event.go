package entstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jensholdgaard/discord-dkp-bot/internal/event"
)

// EventStore implements event.Store using database/sql.
type EventStore struct {
	db *sql.DB
}

// NewEventStore returns a new EventStore.
func NewEventStore(db *sql.DB) *EventStore {
	return &EventStore{db: db}
}

func (s *EventStore) Append(ctx context.Context, events ...event.Event) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx,
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
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, aggregate_id, type, data, version, created_at
		 FROM events WHERE aggregate_id = $1 ORDER BY version ASC`, aggregateID)
	if err != nil {
		return nil, fmt.Errorf("loading events: %w", err)
	}
	defer rows.Close()

	var events []event.Event
	for rows.Next() {
		var e event.Event
		var data []byte
		var createdAt time.Time
		if err := rows.Scan(&e.ID, &e.AggregateID, &e.Type, &data, &e.Version, &createdAt); err != nil {
			return nil, fmt.Errorf("scanning event row: %w", err)
		}
		e.Data = json.RawMessage(data)
		e.CreatedAt = createdAt
		events = append(events, e)
	}
	return events, rows.Err()
}

func (s *EventStore) LoadByType(ctx context.Context, eventType event.Type) ([]event.Event, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, aggregate_id, type, data, version, created_at
		 FROM events WHERE type = $1 ORDER BY created_at ASC`, eventType)
	if err != nil {
		return nil, fmt.Errorf("loading events by type: %w", err)
	}
	defer rows.Close()

	var events []event.Event
	for rows.Next() {
		var e event.Event
		var data []byte
		var createdAt time.Time
		if err := rows.Scan(&e.ID, &e.AggregateID, &e.Type, &data, &e.Version, &createdAt); err != nil {
			return nil, fmt.Errorf("scanning event row: %w", err)
		}
		e.Data = json.RawMessage(data)
		e.CreatedAt = createdAt
		events = append(events, e)
	}
	return events, rows.Err()
}

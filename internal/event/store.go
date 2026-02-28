package event

import "context"

// Store persists and retrieves events.
type Store interface {
	// Append persists one or more events atomically.
	Append(ctx context.Context, events ...Event) error
	// Load returns all events for an aggregate, ordered by version.
	Load(ctx context.Context, aggregateID string) ([]Event, error)
	// LoadByType returns events filtered by type.
	LoadByType(ctx context.Context, eventType Type) ([]Event, error)
}

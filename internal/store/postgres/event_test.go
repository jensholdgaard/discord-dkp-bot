package postgres_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jensholdgaard/discord-dkp-bot/internal/event"
	"github.com/jensholdgaard/discord-dkp-bot/internal/store/postgres"
)

func TestEventStore_AppendAndLoad(t *testing.T) {
	db := newTestDB(t)
	es := postgres.NewEventStore(db)
	ctx := context.Background()

	aggID := "auction-001"
	events := []event.Event{
		{AggregateID: aggID, Type: event.AuctionStarted, Data: json.RawMessage(`{"item_name":"Sword"}`), Version: 1},
		{AggregateID: aggID, Type: event.AuctionBidPlaced, Data: json.RawMessage(`{"player_id":"p1","amount":100}`), Version: 2},
	}

	if err := es.Append(ctx, events...); err != nil {
		t.Fatalf("Append: %v", err)
	}

	loaded, err := es.Load(ctx, aggID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("Load returned %d events, want 2", len(loaded))
	}

	// Should be ordered by version.
	if loaded[0].Version != 1 || loaded[1].Version != 2 {
		t.Errorf("versions = [%d, %d], want [1, 2]", loaded[0].Version, loaded[1].Version)
	}
	if loaded[0].Type != event.AuctionStarted {
		t.Errorf("event[0].Type = %q, want %q", loaded[0].Type, event.AuctionStarted)
	}
}

func TestEventStore_LoadByType(t *testing.T) {
	db := newTestDB(t)
	es := postgres.NewEventStore(db)
	ctx := context.Background()

	events := []event.Event{
		{AggregateID: "a1", Type: event.AuctionStarted, Data: json.RawMessage(`{}`), Version: 1},
		{AggregateID: "a1", Type: event.AuctionBidPlaced, Data: json.RawMessage(`{}`), Version: 2},
		{AggregateID: "a2", Type: event.AuctionStarted, Data: json.RawMessage(`{}`), Version: 1},
	}

	if err := es.Append(ctx, events...); err != nil {
		t.Fatalf("Append: %v", err)
	}

	started, err := es.LoadByType(ctx, event.AuctionStarted)
	if err != nil {
		t.Fatalf("LoadByType: %v", err)
	}
	if len(started) != 2 {
		t.Fatalf("LoadByType(AuctionStarted) returned %d, want 2", len(started))
	}

	bids, err := es.LoadByType(ctx, event.AuctionBidPlaced)
	if err != nil {
		t.Fatalf("LoadByType: %v", err)
	}
	if len(bids) != 1 {
		t.Fatalf("LoadByType(AuctionBidPlaced) returned %d, want 1", len(bids))
	}
}

func TestEventStore_UniqueAggregateVersion(t *testing.T) {
	db := newTestDB(t)
	es := postgres.NewEventStore(db)
	ctx := context.Background()

	e := event.Event{
		AggregateID: "dup-test",
		Type:        event.DKPAwarded,
		Data:        json.RawMessage(`{}`),
		Version:     1,
	}

	if err := es.Append(ctx, e); err != nil {
		t.Fatalf("first Append: %v", err)
	}

	// Duplicate version for the same aggregate should fail.
	err := es.Append(ctx, e)
	if err == nil {
		t.Fatal("expected error for duplicate aggregate_id + version")
	}
}

func TestEventStore_LoadEmpty(t *testing.T) {
	db := newTestDB(t)
	es := postgres.NewEventStore(db)
	ctx := context.Background()

	loaded, err := es.Load(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded) != 0 {
		t.Errorf("expected empty slice, got %d events", len(loaded))
	}
}

package auction_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace/noop"

	"github.com/jensholdgaard/discord-dkp-bot/internal/auction"
	"github.com/jensholdgaard/discord-dkp-bot/internal/clock"
	"github.com/jensholdgaard/discord-dkp-bot/internal/event"
	"github.com/jensholdgaard/discord-dkp-bot/internal/store"
)

// --- mock helpers ---

type mockEventStore struct {
	events   []event.Event
	appendFn func(events ...event.Event) error
}

func (m *mockEventStore) Append(_ context.Context, events ...event.Event) error {
	if m.appendFn != nil {
		return m.appendFn(events...)
	}
	m.events = append(m.events, events...)
	return nil
}

func (m *mockEventStore) Load(_ context.Context, aggregateID string) ([]event.Event, error) {
	var result []event.Event
	for _, e := range m.events {
		if e.AggregateID == aggregateID {
			result = append(result, e)
		}
	}
	return result, nil
}

func (m *mockEventStore) LoadByType(_ context.Context, eventType event.Type) ([]event.Event, error) {
	var result []event.Event
	for _, e := range m.events {
		if e.Type == eventType {
			result = append(result, e)
		}
	}
	return result, nil
}

type mockPlayerRepo struct {
	players map[string]*store.Player
	err     error
}

func newMockPlayerRepo() *mockPlayerRepo {
	return &mockPlayerRepo{players: make(map[string]*store.Player)}
}

func (m *mockPlayerRepo) Create(_ context.Context, p *store.Player) error {
	if m.err != nil {
		return m.err
	}
	p.ID = "test-id-" + p.DiscordID
	m.players[p.DiscordID] = p
	return nil
}

func (m *mockPlayerRepo) GetByDiscordID(_ context.Context, discordID string) (*store.Player, error) {
	if m.err != nil {
		return nil, m.err
	}
	p, ok := m.players[discordID]
	if !ok {
		return nil, fmt.Errorf("player not found")
	}
	return p, nil
}

func (m *mockPlayerRepo) GetByCharacterName(_ context.Context, name string) (*store.Player, error) {
	for _, p := range m.players {
		if p.CharacterName == name {
			return p, nil
		}
	}
	return nil, fmt.Errorf("player not found")
}

func (m *mockPlayerRepo) List(_ context.Context) ([]store.Player, error) {
	result := make([]store.Player, 0, len(m.players))
	for _, p := range m.players {
		result = append(result, *p)
	}
	return result, nil
}

func (m *mockPlayerRepo) UpdateDKP(_ context.Context, id string, delta int) error {
	if m.err != nil {
		return m.err
	}
	for _, p := range m.players {
		if p.ID == id {
			p.DKP += delta
			return nil
		}
	}
	return fmt.Errorf("player %s not found", id)
}

// --- tests ---

func TestManager_StartAuction(t *testing.T) {
	es := &mockEventStore{}
	repo := newMockPlayerRepo()
	tp := noop.NewTracerProvider()
	clk := clock.Mock{T: time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)}
	logger := slog.Default()

	mgr := auction.NewManager(es, repo, logger, tp, clk)

	a, err := mgr.StartAuction(context.Background(), "Legendary Sword", "admin", 10, 5*time.Minute)
	if err != nil {
		t.Fatalf("StartAuction() error = %v", err)
	}
	if a == nil {
		t.Fatal("StartAuction() returned nil auction")
	}
	if a.ItemName != "Legendary Sword" {
		t.Errorf("ItemName = %q, want %q", a.ItemName, "Legendary Sword")
	}
	if a.Status != "open" {
		t.Errorf("Status = %q, want %q", a.Status, "open")
	}
	if len(es.events) == 0 {
		t.Error("expected events to be persisted")
	}
}

func TestManager_StartAuction_PersistError(t *testing.T) {
	es := &mockEventStore{
		appendFn: func(events ...event.Event) error {
			return fmt.Errorf("db write error")
		},
	}
	repo := newMockPlayerRepo()
	tp := noop.NewTracerProvider()
	clk := clock.Mock{T: time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)}
	logger := slog.Default()

	mgr := auction.NewManager(es, repo, logger, tp, clk)

	_, err := mgr.StartAuction(context.Background(), "Sword", "admin", 10, 5*time.Minute)
	if err == nil {
		t.Fatal("expected error when event store fails")
	}
}

func TestManager_PlaceBid(t *testing.T) {
	es := &mockEventStore{}
	repo := newMockPlayerRepo()
	tp := noop.NewTracerProvider()
	clk := clock.Mock{T: time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)}
	logger := slog.Default()

	// Register a player.
	repo.players["discord-1"] = &store.Player{
		ID:        "player-1",
		DiscordID: "discord-1",
		DKP:       200,
	}

	mgr := auction.NewManager(es, repo, logger, tp, clk)

	a, _ := mgr.StartAuction(context.Background(), "Shield", "admin", 10, 5*time.Minute)

	err := mgr.PlaceBid(context.Background(), a.ID, "discord-1", 50)
	if err != nil {
		t.Fatalf("PlaceBid() error = %v", err)
	}

	highest := a.HighestBid()
	if highest == nil || highest.Amount != 50 {
		t.Errorf("highest bid = %+v, want amount=50", highest)
	}
}

func TestManager_PlaceBid_AuctionNotFound(t *testing.T) {
	es := &mockEventStore{}
	repo := newMockPlayerRepo()
	tp := noop.NewTracerProvider()
	clk := clock.Mock{T: time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)}
	logger := slog.Default()

	mgr := auction.NewManager(es, repo, logger, tp, clk)

	err := mgr.PlaceBid(context.Background(), "nonexistent", "discord-1", 50)
	if err == nil {
		t.Fatal("expected error for nonexistent auction")
	}
}

func TestManager_PlaceBid_PlayerNotRegistered(t *testing.T) {
	es := &mockEventStore{}
	repo := newMockPlayerRepo()
	tp := noop.NewTracerProvider()
	clk := clock.Mock{T: time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)}
	logger := slog.Default()

	mgr := auction.NewManager(es, repo, logger, tp, clk)

	a, _ := mgr.StartAuction(context.Background(), "Shield", "admin", 10, 5*time.Minute)

	err := mgr.PlaceBid(context.Background(), a.ID, "unknown-discord", 50)
	if err == nil {
		t.Fatal("expected error for unregistered player")
	}
}

func TestManager_CloseAuction(t *testing.T) {
	es := &mockEventStore{}
	repo := newMockPlayerRepo()
	tp := noop.NewTracerProvider()
	clk := clock.Mock{T: time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)}
	logger := slog.Default()

	repo.players["discord-1"] = &store.Player{
		ID:        "player-1",
		DiscordID: "discord-1",
		DKP:       200,
	}

	mgr := auction.NewManager(es, repo, logger, tp, clk)

	a, _ := mgr.StartAuction(context.Background(), "Helm", "admin", 10, 5*time.Minute)
	_ = mgr.PlaceBid(context.Background(), a.ID, "discord-1", 75)

	msg, err := mgr.CloseAuction(context.Background(), a.ID)
	if err != nil {
		t.Fatalf("CloseAuction() error = %v", err)
	}
	if msg == "" {
		t.Error("expected a winner message, got empty string")
	}
}

func TestManager_CloseAuction_NoBids(t *testing.T) {
	es := &mockEventStore{}
	repo := newMockPlayerRepo()
	tp := noop.NewTracerProvider()
	clk := clock.Mock{T: time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)}
	logger := slog.Default()

	mgr := auction.NewManager(es, repo, logger, tp, clk)

	a, _ := mgr.StartAuction(context.Background(), "Empty Auction", "admin", 10, 5*time.Minute)

	msg, err := mgr.CloseAuction(context.Background(), a.ID)
	if err != nil {
		t.Fatalf("CloseAuction() error = %v", err)
	}
	if msg != "" {
		t.Errorf("expected empty message for no-bid close, got %q", msg)
	}
}

func TestManager_CloseAuction_NotFound(t *testing.T) {
	es := &mockEventStore{}
	repo := newMockPlayerRepo()
	tp := noop.NewTracerProvider()
	clk := clock.Mock{T: time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)}
	logger := slog.Default()

	mgr := auction.NewManager(es, repo, logger, tp, clk)

	_, err := mgr.CloseAuction(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent auction")
	}
}

func TestManager_ReplayAuction(t *testing.T) {
	es := &mockEventStore{}
	repo := newMockPlayerRepo()
	tp := noop.NewTracerProvider()
	clk := clock.Mock{T: time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)}
	logger := slog.Default()

	repo.players["discord-1"] = &store.Player{
		ID:        "player-1",
		DiscordID: "discord-1",
		DKP:       500,
	}

	mgr := auction.NewManager(es, repo, logger, tp, clk)

	a, _ := mgr.StartAuction(context.Background(), "Replay Item", "admin", 10, 5*time.Minute)
	_ = mgr.PlaceBid(context.Background(), a.ID, "discord-1", 100)

	replayed, err := mgr.ReplayAuction(context.Background(), a.ID)
	if err != nil {
		t.Fatalf("ReplayAuction() error = %v", err)
	}
	if replayed.ItemName != "Replay Item" {
		t.Errorf("ItemName = %q, want %q", replayed.ItemName, "Replay Item")
	}
	if len(replayed.Bids) != 1 {
		t.Errorf("bids = %d, want 1", len(replayed.Bids))
	}
}

func TestAuction_Cancel(t *testing.T) {
	tp := noop.NewTracerProvider()
	clk := clock.Mock{T: time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)}

	a := auction.New("cancel-test", "Ring", "admin", 10, 5*time.Minute, tp, clk)

	if err := a.Cancel(context.Background()); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if a.Status != "canceled" {
		t.Errorf("Status = %q, want %q", a.Status, "canceled")
	}

	// Cancel again should fail.
	if err := a.Cancel(context.Background()); err != auction.ErrAuctionClosed {
		t.Errorf("Cancel() on canceled auction error = %v, want ErrAuctionClosed", err)
	}
}

func TestAuction_Cancel_AlreadyClosed(t *testing.T) {
	tp := noop.NewTracerProvider()
	clk := clock.Mock{T: time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)}

	a := auction.New("cancel-closed-test", "Gem", "admin", 10, 5*time.Minute, tp, clk)
	_, _ = a.Close(context.Background())

	err := a.Cancel(context.Background())
	if err != auction.ErrAuctionClosed {
		t.Errorf("Cancel() on closed auction error = %v, want ErrAuctionClosed", err)
	}
}

func TestReplay_EmptyEvents(t *testing.T) {
	_, err := auction.Replay(nil)
	if err == nil {
		t.Fatal("expected error for empty events")
	}
}

func TestReplay_CancelledStatus(t *testing.T) {
	tp := noop.NewTracerProvider()
	clk := clock.Mock{T: time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)}

	a := auction.New("replay-cancel", "Wand", "admin", 10, 5*time.Minute, tp, clk)
	_ = a.Cancel(context.Background())

	events := a.PendingEvents()

	replayed, err := auction.Replay(events)
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if replayed.Status != "canceled" {
		t.Errorf("Status = %q, want %q", replayed.Status, "canceled")
	}
}

func TestReplay_ClosedStatus(t *testing.T) {
	tp := noop.NewTracerProvider()
	clk := clock.Mock{T: time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)}

	a := auction.New("replay-close", "Staff", "admin", 10, 5*time.Minute, tp, clk)
	_ = a.PlaceBid(context.Background(), "p1", 50, 100)
	_, _ = a.Close(context.Background())

	events := a.PendingEvents()

	replayed, err := auction.Replay(events)
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if replayed.Status != "closed" {
		t.Errorf("Status = %q, want %q", replayed.Status, "closed")
	}
}

func TestReplay_InvalidStartedData(t *testing.T) {
	events := []event.Event{
		{
			AggregateID: "bad",
			Type:        event.AuctionStarted,
			Data:        json.RawMessage(`{invalid`),
			Version:     1,
		},
	}
	_, err := auction.Replay(events)
	if err == nil {
		t.Fatal("expected error for invalid started event data")
	}
}

func TestReplay_InvalidBidData(t *testing.T) {
	startData, _ := json.Marshal(event.AuctionStartedData{
		ItemName:  "Sword",
		StartedBy: "admin",
		MinBid:    10,
		Duration:  5 * time.Minute,
	})
	events := []event.Event{
		{
			AggregateID: "bad-bid",
			Type:        event.AuctionStarted,
			Data:        startData,
			Version:     1,
		},
		{
			AggregateID: "bad-bid",
			Type:        event.AuctionBidPlaced,
			Data:        json.RawMessage(`{invalid`),
			Version:     2,
		},
	}
	_, err := auction.Replay(events)
	if err == nil {
		t.Fatal("expected error for invalid bid event data")
	}
}

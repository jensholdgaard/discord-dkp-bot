package dkp_test

import (
	"context"
	"fmt"
	"log/slog"
	"testing"

	"go.opentelemetry.io/otel/trace/noop"

	"github.com/jensholdgaard/discord-dkp-bot/internal/dkp"
	"github.com/jensholdgaard/discord-dkp-bot/internal/event"
	"github.com/jensholdgaard/discord-dkp-bot/internal/store"
)

var testTP = noop.NewTracerProvider()

// mockPlayerRepo implements store.PlayerRepository for testing.
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

// mockEventStore implements event.Store for testing.
type mockEventStore struct {
	events []event.Event
}

func (m *mockEventStore) Append(_ context.Context, events ...event.Event) error {
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

func TestManager_RegisterPlayer(t *testing.T) {
	tests := []struct {
		name          string
		discordID     string
		characterName string
		wantErr       bool
	}{
		{
			name:          "successful registration",
			discordID:     "discord-123",
			characterName: "Gandalf",
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockPlayerRepo()
			es := &mockEventStore{}
			logger := slog.Default()
			mgr := dkp.NewManager(repo, es, logger, testTP)

			p, err := mgr.RegisterPlayer(context.Background(), tt.discordID, tt.characterName)
			if (err != nil) != tt.wantErr {
				t.Fatalf("RegisterPlayer() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if p.CharacterName != tt.characterName {
					t.Errorf("character = %q, want %q", p.CharacterName, tt.characterName)
				}
				if len(es.events) != 1 {
					t.Errorf("events = %d, want 1", len(es.events))
				}
			}
		})
	}
}

func TestManager_AwardDKP(t *testing.T) {
	tests := []struct {
		name    string
		amount  int
		reason  string
		wantDKP int
		wantErr bool
	}{
		{
			name:    "award 50 DKP",
			amount:  50,
			reason:  "raid attendance",
			wantDKP: 50,
		},
		{
			name:    "award 100 DKP",
			amount:  100,
			reason:  "boss kill",
			wantDKP: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockPlayerRepo()
			es := &mockEventStore{}
			logger := slog.Default()
			mgr := dkp.NewManager(repo, es, logger, testTP)

			// Register player first.
			p, _ := mgr.RegisterPlayer(context.Background(), "d1", "Legolas")

			err := mgr.AwardDKP(context.Background(), p.ID, tt.amount, tt.reason)
			if (err != nil) != tt.wantErr {
				t.Fatalf("AwardDKP() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && p.DKP != tt.wantDKP {
				t.Errorf("DKP = %d, want %d", p.DKP, tt.wantDKP)
			}
		})
	}
}

func TestManager_DeductDKP(t *testing.T) {
	repo := newMockPlayerRepo()
	es := &mockEventStore{}
	logger := slog.Default()
	mgr := dkp.NewManager(repo, es, logger, testTP)

	p, _ := mgr.RegisterPlayer(context.Background(), "d1", "Aragorn")
	_ = mgr.AwardDKP(context.Background(), p.ID, 100, "seed")

	err := mgr.DeductDKP(context.Background(), p.ID, 30, "item purchased")
	if err != nil {
		t.Fatalf("DeductDKP() error: %v", err)
	}
	if p.DKP != 70 {
		t.Errorf("DKP = %d, want 70", p.DKP)
	}
}

func TestManager_GetPlayer(t *testing.T) {
	repo := newMockPlayerRepo()
	es := &mockEventStore{}
	logger := slog.Default()
	mgr := dkp.NewManager(repo, es, logger, testTP)

	_, _ = mgr.RegisterPlayer(context.Background(), "d-get", "Frodo")

	p, err := mgr.GetPlayer(context.Background(), "d-get")
	if err != nil {
		t.Fatalf("GetPlayer() error = %v", err)
	}
	if p.CharacterName != "Frodo" {
		t.Errorf("CharacterName = %q, want %q", p.CharacterName, "Frodo")
	}
}

func TestManager_GetPlayer_NotFound(t *testing.T) {
	repo := newMockPlayerRepo()
	es := &mockEventStore{}
	logger := slog.Default()
	mgr := dkp.NewManager(repo, es, logger, testTP)

	_, err := mgr.GetPlayer(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent player")
	}
}

func TestManager_ListPlayers(t *testing.T) {
	repo := newMockPlayerRepo()
	es := &mockEventStore{}
	logger := slog.Default()
	mgr := dkp.NewManager(repo, es, logger, testTP)

	_, _ = mgr.RegisterPlayer(context.Background(), "d1", "Sam")
	_, _ = mgr.RegisterPlayer(context.Background(), "d2", "Pippin")

	players, err := mgr.ListPlayers(context.Background())
	if err != nil {
		t.Fatalf("ListPlayers() error = %v", err)
	}
	if len(players) != 2 {
		t.Errorf("players count = %d, want 2", len(players))
	}
}

func TestManager_RegisterPlayer_RepoError(t *testing.T) {
	repo := newMockPlayerRepo()
	repo.err = fmt.Errorf("db error")
	es := &mockEventStore{}
	logger := slog.Default()
	mgr := dkp.NewManager(repo, es, logger, testTP)

	_, err := mgr.RegisterPlayer(context.Background(), "d1", "Boromir")
	if err == nil {
		t.Fatal("expected error when repo returns error")
	}
}

func TestManager_AwardDKP_PlayerNotFound(t *testing.T) {
	repo := newMockPlayerRepo()
	es := &mockEventStore{}
	logger := slog.Default()
	mgr := dkp.NewManager(repo, es, logger, testTP)

	err := mgr.AwardDKP(context.Background(), "nonexistent-id", 50, "test")
	if err == nil {
		t.Fatal("expected error when player not found")
	}
}

func TestManager_DeductDKP_PlayerNotFound(t *testing.T) {
	repo := newMockPlayerRepo()
	es := &mockEventStore{}
	logger := slog.Default()
	mgr := dkp.NewManager(repo, es, logger, testTP)

	err := mgr.DeductDKP(context.Background(), "nonexistent-id", 30, "test")
	if err == nil {
		t.Fatal("expected error when player not found")
	}
}

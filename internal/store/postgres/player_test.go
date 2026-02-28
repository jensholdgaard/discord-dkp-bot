package postgres_test

import (
	"context"
	"testing"

	"github.com/jensholdgaard/discord-dkp-bot/internal/clock"
	"github.com/jensholdgaard/discord-dkp-bot/internal/store"
	"github.com/jensholdgaard/discord-dkp-bot/internal/store/postgres"
)

func TestPlayerRepo_CreateAndGet(t *testing.T) {
	db := newTestDB(t)
	repo := postgres.NewPlayerRepo(db, clock.Real{})
	ctx := context.Background()

	p := &store.Player{
		DiscordID:     "discord-123",
		CharacterName: "TestChar",
		DKP:           100,
	}

	if err := repo.Create(ctx, p); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if p.ID == "" {
		t.Fatal("expected ID to be set after Create")
	}

	// GetByDiscordID
	got, err := repo.GetByDiscordID(ctx, "discord-123")
	if err != nil {
		t.Fatalf("GetByDiscordID: %v", err)
	}
	if got.CharacterName != "TestChar" {
		t.Errorf("CharacterName = %q, want %q", got.CharacterName, "TestChar")
	}
	if got.DKP != 100 {
		t.Errorf("DKP = %d, want %d", got.DKP, 100)
	}

	// GetByCharacterName
	got2, err := repo.GetByCharacterName(ctx, "TestChar")
	if err != nil {
		t.Fatalf("GetByCharacterName: %v", err)
	}
	if got2.DiscordID != "discord-123" {
		t.Errorf("DiscordID = %q, want %q", got2.DiscordID, "discord-123")
	}
}

func TestPlayerRepo_List(t *testing.T) {
	db := newTestDB(t)
	repo := postgres.NewPlayerRepo(db, clock.Real{})
	ctx := context.Background()

	// Create two players.
	for _, p := range []*store.Player{
		{DiscordID: "d1", CharacterName: "Alpha", DKP: 50},
		{DiscordID: "d2", CharacterName: "Bravo", DKP: 200},
	} {
		if err := repo.Create(ctx, p); err != nil {
			t.Fatalf("Create(%s): %v", p.CharacterName, err)
		}
	}

	players, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(players) != 2 {
		t.Fatalf("List returned %d players, want 2", len(players))
	}

	// Ordered by DKP DESC, so Bravo (200) should be first.
	if players[0].CharacterName != "Bravo" {
		t.Errorf("first player = %q, want %q", players[0].CharacterName, "Bravo")
	}
}

func TestPlayerRepo_UpdateDKP(t *testing.T) {
	db := newTestDB(t)
	repo := postgres.NewPlayerRepo(db, clock.Real{})
	ctx := context.Background()

	p := &store.Player{DiscordID: "d1", CharacterName: "DKPTest", DKP: 100}
	if err := repo.Create(ctx, p); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Award +50
	if err := repo.UpdateDKP(ctx, p.ID, 50); err != nil {
		t.Fatalf("UpdateDKP(+50): %v", err)
	}

	got, _ := repo.GetByDiscordID(ctx, "d1")
	if got.DKP != 150 {
		t.Errorf("DKP after +50 = %d, want 150", got.DKP)
	}

	// Deduct -30
	if err := repo.UpdateDKP(ctx, p.ID, -30); err != nil {
		t.Fatalf("UpdateDKP(-30): %v", err)
	}

	got, _ = repo.GetByDiscordID(ctx, "d1")
	if got.DKP != 120 {
		t.Errorf("DKP after -30 = %d, want 120", got.DKP)
	}
}

func TestPlayerRepo_UpdateDKP_NotFound(t *testing.T) {
	db := newTestDB(t)
	repo := postgres.NewPlayerRepo(db, clock.Real{})
	ctx := context.Background()

	err := repo.UpdateDKP(ctx, "00000000-0000-0000-0000-000000000000", 10)
	if err == nil {
		t.Fatal("expected error for nonexistent player")
	}
}

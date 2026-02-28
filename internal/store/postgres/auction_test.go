package postgres_test

import (
	"context"
	"testing"

	"github.com/jensholdgaard/discord-dkp-bot/internal/clock"
	"github.com/jensholdgaard/discord-dkp-bot/internal/store"
	"github.com/jensholdgaard/discord-dkp-bot/internal/store/postgres"
)

func TestAuctionRepo_CreateAndGetByID(t *testing.T) {
	db := newTestDB(t)
	repo := postgres.NewAuctionRepo(db, clock.Real{})
	ctx := context.Background()

	a := &store.Auction{
		ItemName:  "Thunderfury",
		StartedBy: "gm-1",
		MinBid:    50,
	}
	if err := repo.Create(ctx, a); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if a.ID == "" {
		t.Fatal("expected ID to be set after Create")
	}
	if a.Status != "open" {
		t.Errorf("Status = %q, want %q", a.Status, "open")
	}

	got, err := repo.GetByID(ctx, a.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ItemName != "Thunderfury" {
		t.Errorf("ItemName = %q, want %q", got.ItemName, "Thunderfury")
	}
}

func TestAuctionRepo_ListOpen(t *testing.T) {
	db := newTestDB(t)
	repo := postgres.NewAuctionRepo(db, clock.Real{})
	ctx := context.Background()

	for _, item := range []string{"Item1", "Item2"} {
		a := &store.Auction{ItemName: item, StartedBy: "gm", MinBid: 10}
		if err := repo.Create(ctx, a); err != nil {
			t.Fatalf("Create(%s): %v", item, err)
		}
	}

	open, err := repo.ListOpen(ctx)
	if err != nil {
		t.Fatalf("ListOpen: %v", err)
	}
	if len(open) != 2 {
		t.Fatalf("ListOpen returned %d, want 2", len(open))
	}
}

func TestAuctionRepo_Close(t *testing.T) {
	db := newTestDB(t)
	clk := clock.Real{}
	auctionRepo := postgres.NewAuctionRepo(db, clk)
	playerRepo := postgres.NewPlayerRepo(db, clk)
	ctx := context.Background()

	// Need a real player for the winner foreign key.
	p := &store.Player{DiscordID: "winner-1", CharacterName: "Winner", DKP: 500}
	if err := playerRepo.Create(ctx, p); err != nil {
		t.Fatalf("Create player: %v", err)
	}

	a := &store.Auction{ItemName: "Sword", StartedBy: "gm", MinBid: 10}
	if err := auctionRepo.Create(ctx, a); err != nil {
		t.Fatalf("Create auction: %v", err)
	}

	if err := auctionRepo.Close(ctx, a.ID, p.ID, 200); err != nil {
		t.Fatalf("Close: %v", err)
	}

	got, _ := auctionRepo.GetByID(ctx, a.ID)
	if got.Status != "closed" {
		t.Errorf("Status = %q, want %q", got.Status, "closed")
	}
	if got.WinnerID == nil || *got.WinnerID != p.ID {
		t.Errorf("WinnerID = %v, want %q", got.WinnerID, p.ID)
	}
	if got.WinAmount == nil || *got.WinAmount != 200 {
		t.Errorf("WinAmount = %v, want 200", got.WinAmount)
	}
	if got.ClosedAt == nil {
		t.Error("expected ClosedAt to be set")
	}

	// Closing again should fail.
	if err := auctionRepo.Close(ctx, a.ID, p.ID, 300); err == nil {
		t.Error("expected error closing an already-closed auction")
	}
}

func TestAuctionRepo_Cancel(t *testing.T) {
	db := newTestDB(t)
	repo := postgres.NewAuctionRepo(db, clock.Real{})
	ctx := context.Background()

	a := &store.Auction{ItemName: "Shield", StartedBy: "gm", MinBid: 5}
	if err := repo.Create(ctx, a); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := repo.Cancel(ctx, a.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	got, _ := repo.GetByID(ctx, a.ID)
	if got.Status != "cancelled" {
		t.Errorf("Status = %q, want %q", got.Status, "cancelled")
	}

	// Should not appear in open list.
	open, _ := repo.ListOpen(ctx)
	if len(open) != 0 {
		t.Errorf("ListOpen returned %d after cancel, want 0", len(open))
	}

	// Cancelling again should fail.
	if err := repo.Cancel(ctx, a.ID); err == nil {
		t.Error("expected error cancelling an already-cancelled auction")
	}
}

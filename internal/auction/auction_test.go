package auction_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace/noop"

	"github.com/jensholdgaard/discord-dkp-bot/internal/auction"
	"github.com/jensholdgaard/discord-dkp-bot/internal/clock"
)

var (
	testTP  = noop.NewTracerProvider()
	testClk = clock.Real{}
)

func TestPlaceBid(t *testing.T) {
	tests := []struct {
		name      string
		setup     func() *auction.Auction
		playerID  string
		amount    int
		playerDKP int
		wantErr   error
	}{
		{
			name: "valid first bid",
			setup: func() *auction.Auction {
				return auction.New("a1", "Sword of Truth", "admin", 10, 5*time.Minute, testTP, testClk)
			},
			playerID:  "p1",
			amount:    50,
			playerDKP: 100,
			wantErr:   nil,
		},
		{
			name: "bid below minimum",
			setup: func() *auction.Auction {
				return auction.New("a2", "Shield", "admin", 100, 5*time.Minute, testTP, testClk)
			},
			playerID:  "p1",
			amount:    50,
			playerDKP: 200,
			wantErr:   auction.ErrBidTooLow,
		},
		{
			name: "insufficient DKP",
			setup: func() *auction.Auction {
				return auction.New("a3", "Helm", "admin", 10, 5*time.Minute, testTP, testClk)
			},
			playerID:  "p1",
			amount:    150,
			playerDKP: 100,
			wantErr:   auction.ErrInsufficientDKP,
		},
		{
			name: "self outbid",
			setup: func() *auction.Auction {
				a := auction.New("a4", "Boots", "admin", 10, 5*time.Minute, testTP, testClk)
				_ = a.PlaceBid(context.Background(), "p1", 50, 100)
				return a
			},
			playerID:  "p1",
			amount:    60,
			playerDKP: 100,
			wantErr:   auction.ErrSelfOutbid,
		},
		{
			name: "bid on closed auction",
			setup: func() *auction.Auction {
				a := auction.New("a5", "Ring", "admin", 10, 5*time.Minute, testTP, testClk)
				_, _ = a.Close(context.Background())
				return a
			},
			playerID:  "p1",
			amount:    50,
			playerDKP: 100,
			wantErr:   auction.ErrAuctionClosed,
		},
		{
			name: "must outbid current highest",
			setup: func() *auction.Auction {
				a := auction.New("a6", "Cloak", "admin", 10, 5*time.Minute, testTP, testClk)
				_ = a.PlaceBid(context.Background(), "p1", 50, 100)
				return a
			},
			playerID:  "p2",
			amount:    30,
			playerDKP: 100,
			wantErr:   auction.ErrBidTooLow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := tt.setup()
			err := a.PlaceBid(context.Background(), tt.playerID, tt.amount, tt.playerDKP)
			if err != tt.wantErr {
				t.Errorf("PlaceBid() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAuction_Close(t *testing.T) {
	tests := []struct {
		name       string
		setup      func() *auction.Auction
		wantWinner bool
		wantErr    error
	}{
		{
			name: "close with winner",
			setup: func() *auction.Auction {
				a := auction.New("a1", "Sword", "admin", 10, 5*time.Minute, testTP, testClk)
				_ = a.PlaceBid(context.Background(), "p1", 50, 100)
				_ = a.PlaceBid(context.Background(), "p2", 75, 200)
				return a
			},
			wantWinner: true,
		},
		{
			name: "close with no bids",
			setup: func() *auction.Auction {
				return auction.New("a2", "Shield", "admin", 10, 5*time.Minute, testTP, testClk)
			},
			wantWinner: false,
		},
		{
			name: "close already closed",
			setup: func() *auction.Auction {
				a := auction.New("a3", "Helm", "admin", 10, 5*time.Minute, testTP, testClk)
				_, _ = a.Close(context.Background())
				return a
			},
			wantErr: auction.ErrAuctionClosed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := tt.setup()
			winner, err := a.Close(context.Background())
			if err != tt.wantErr {
				t.Fatalf("Close() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantWinner && winner == nil {
				t.Error("expected a winner, got nil")
			}
			if !tt.wantWinner && winner != nil && tt.wantErr == nil {
				t.Errorf("expected no winner, got %+v", winner)
			}
		})
	}
}

func TestAuction_ConcurrentBids(t *testing.T) {
	a := auction.New("concurrent-test", "Epic Item", "admin", 1, 5*time.Minute, testTP, testClk)

	var wg sync.WaitGroup
	errs := make([]error, 100)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			playerID := fmt.Sprintf("player-%d", idx)
			errs[idx] = a.PlaceBid(context.Background(), playerID, idx+1, 1000)
		}(i)
	}
	wg.Wait()

	// At least one bid should have succeeded.
	var successCount int
	for _, err := range errs {
		if err == nil {
			successCount++
		}
	}
	if successCount == 0 {
		t.Error("expected at least one successful bid in concurrent scenario")
	}

	// Verify highest bid is consistent.
	highest := a.HighestBid()
	if highest == nil {
		t.Fatal("expected a highest bid")
	}
}

func TestAuction_Replay(t *testing.T) {
	// Create auction and place bids.
	original := auction.New("replay-test", "Legendary Sword", "admin", 10, 5*time.Minute, testTP, testClk)
	_ = original.PlaceBid(context.Background(), "p1", 50, 100)
	_ = original.PlaceBid(context.Background(), "p2", 75, 200)

	events := original.PendingEvents()

	// Replay from events.
	replayed, err := auction.Replay(events)
	if err != nil {
		t.Fatalf("Replay() error: %v", err)
	}

	if replayed.ItemName != original.ItemName {
		t.Errorf("item name = %q, want %q", replayed.ItemName, original.ItemName)
	}
	if replayed.Status != "open" {
		t.Errorf("status = %q, want %q", replayed.Status, "open")
	}
	if len(replayed.Bids) != 2 {
		t.Errorf("bids count = %d, want 2", len(replayed.Bids))
	}

	highest := replayed.HighestBid()
	if highest == nil || highest.PlayerID != "p2" || highest.Amount != 75 {
		t.Errorf("highest bid = %+v, want p2 @ 75", highest)
	}
}

func TestAuction_PendingEvents(t *testing.T) {
	a := auction.New("events-test", "Item", "admin", 10, 5*time.Minute, testTP, testClk)
	_ = a.PlaceBid(context.Background(), "p1", 50, 100)

	events := a.PendingEvents()
	if len(events) != 2 { // started + bid
		t.Errorf("pending events = %d, want 2", len(events))
	}

	// Should be empty after drain.
	events = a.PendingEvents()
	if len(events) != 0 {
		t.Errorf("pending events after drain = %d, want 0", len(events))
	}
}

package auction

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/jensholdgaard/discord-dkp-bot/internal/clock"
	"github.com/jensholdgaard/discord-dkp-bot/internal/event"
)

// Errors returned by auction operations.
var (
	ErrAuctionClosed   = errors.New("auction is closed")
	ErrBidTooLow       = errors.New("bid is below minimum")
	ErrSelfOutbid      = errors.New("you are already the highest bidder")
	ErrInsufficientDKP = errors.New("insufficient DKP")
)

// Bid represents a single bid in an auction.
type Bid struct {
	PlayerID string
	Amount   int
	Time     time.Time
}

// Auction is the aggregate root for a single item auction.
// It is safe for concurrent use.
type Auction struct {
	mu sync.RWMutex

	ID        string
	ItemName  string
	StartedBy string
	MinBid    int
	Status    string // "open", "closed", "canceled"
	Bids      []Bid
	Version   int

	tracer trace.Tracer
	clock  clock.Clock
	events []event.Event
}

// New creates a new open auction and records a started event.
// The TracerProvider is used to create a scoped tracer for this auction.
func New(id, itemName, startedBy string, minBid int, duration time.Duration, tp trace.TracerProvider, clk clock.Clock) *Auction {
	a := &Auction{
		ID:        id,
		ItemName:  itemName,
		StartedBy: startedBy,
		MinBid:    minBid,
		Status:    "open",
		Version:   0,
		tracer:    tp.Tracer("github.com/jensholdgaard/discord-dkp-bot/internal/auction"),
		clock:     clk,
	}

	data, _ := json.Marshal(event.AuctionStartedData{
		ItemName:  itemName,
		StartedBy: startedBy,
		MinBid:    minBid,
		Duration:  duration,
	})
	a.recordEvent(event.AuctionStarted, data)
	return a
}

// PlaceBid places a bid on the auction. Thread-safe.
func (a *Auction) PlaceBid(ctx context.Context, playerID string, amount int, playerDKP int) error {
	ctx, span := a.tracer.Start(ctx, "Auction.PlaceBid",
		trace.WithAttributes(
			attribute.String("auction.id", a.ID),
			attribute.String("player.id", playerID),
			attribute.Int("bid.amount", amount),
		),
	)
	defer span.End()

	a.mu.Lock()
	defer a.mu.Unlock()

	if a.Status != "open" {
		return ErrAuctionClosed
	}
	if amount < a.MinBid {
		return ErrBidTooLow
	}
	if amount > playerDKP {
		return ErrInsufficientDKP
	}

	// Check if already highest bidder.
	if highest := a.highestBid(); highest != nil && highest.PlayerID == playerID {
		return ErrSelfOutbid
	}

	// Must outbid current highest.
	if highest := a.highestBid(); highest != nil && amount <= highest.Amount {
		return ErrBidTooLow
	}

	a.Bids = append(a.Bids, Bid{
		PlayerID: playerID,
		Amount:   amount,
		Time:     a.clock.Now().UTC(),
	})

	data, _ := json.Marshal(event.BidPlacedData{
		PlayerID: playerID,
		Amount:   amount,
	})
	a.recordEvent(event.AuctionBidPlaced, data)

	slog.InfoContext(ctx, "bid placed",
		slog.String("auction_id", a.ID),
		slog.String("player_id", playerID),
		slog.Int("amount", amount),
	)
	return nil
}

// Close closes the auction, awarding the item to the highest bidder.
func (a *Auction) Close(ctx context.Context) (winner *Bid, err error) {
	_, span := a.tracer.Start(ctx, "Auction.Close",
		trace.WithAttributes(attribute.String("auction.id", a.ID)),
	)
	defer span.End()

	a.mu.Lock()
	defer a.mu.Unlock()

	if a.Status != "open" {
		return nil, ErrAuctionClosed
	}

	a.Status = "closed"
	highest := a.highestBid()

	if highest != nil {
		data, _ := json.Marshal(event.AuctionClosedData{
			WinnerID: highest.PlayerID,
			Amount:   highest.Amount,
		})
		a.recordEvent(event.AuctionClosed, data)
		return highest, nil
	}

	// No bids — close with no winner.
	data, _ := json.Marshal(event.AuctionClosedData{})
	a.recordEvent(event.AuctionClosed, data)
	return nil, nil
}

// Cancel cancels the auction.
func (a *Auction) Cancel(ctx context.Context) error {
	_, span := a.tracer.Start(ctx, "Auction.Cancel",
		trace.WithAttributes(attribute.String("auction.id", a.ID)),
	)
	defer span.End()

	a.mu.Lock()
	defer a.mu.Unlock()

	if a.Status != "open" {
		return ErrAuctionClosed
	}
	a.Status = "canceled"
	a.recordEvent(event.AuctionCancelled, json.RawMessage(`{}`))
	return nil
}

// HighestBid returns the current highest bid (thread-safe).
func (a *Auction) HighestBid() *Bid {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.highestBid()
}

func (a *Auction) highestBid() *Bid {
	if len(a.Bids) == 0 {
		return nil
	}
	return &a.Bids[len(a.Bids)-1]
}

// PendingEvents returns uncommitted events and clears the buffer.
func (a *Auction) PendingEvents() []event.Event {
	a.mu.Lock()
	defer a.mu.Unlock()
	events := a.events
	a.events = nil
	return events
}

func (a *Auction) recordEvent(t event.Type, data json.RawMessage) {
	a.Version++
	a.events = append(a.events, event.Event{
		AggregateID: a.ID,
		Type:        t,
		Data:        data,
		Version:     a.Version,
	})
}

// Replay reconstructs an auction from its event history.
func Replay(events []event.Event) (*Auction, error) {
	if len(events) == 0 {
		return nil, fmt.Errorf("no events to replay")
	}

	a := &Auction{
		tracer: noop.NewTracerProvider().Tracer("auction"),
		clock:  clock.Real{},
	}
	for _, e := range events {
		switch e.Type {
		case event.AuctionStarted:
			var d event.AuctionStartedData
			if err := json.Unmarshal(e.Data, &d); err != nil {
				return nil, fmt.Errorf("unmarshalling started event: %w", err)
			}
			a.ID = e.AggregateID
			a.ItemName = d.ItemName
			a.StartedBy = d.StartedBy
			a.MinBid = d.MinBid
			a.Status = "open"

		case event.AuctionBidPlaced:
			var d event.BidPlacedData
			if err := json.Unmarshal(e.Data, &d); err != nil {
				return nil, fmt.Errorf("unmarshalling bid event: %w", err)
			}
			a.Bids = append(a.Bids, Bid{
				PlayerID: d.PlayerID,
				Amount:   d.Amount,
				Time:     e.CreatedAt,
			})

		case event.AuctionClosed:
			a.Status = "closed"

		case event.AuctionCancelled:
			a.Status = "canceled"
		}
		a.Version = e.Version
	}
	return a, nil
}

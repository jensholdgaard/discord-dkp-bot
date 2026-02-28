package auction

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/jensholdgaard/discord-dkp-bot/internal/clock"
	"github.com/jensholdgaard/discord-dkp-bot/internal/event"
	"github.com/jensholdgaard/discord-dkp-bot/internal/store"
)

// Manager coordinates auction lifecycle and concurrency.
type Manager struct {
	mu       sync.RWMutex
	auctions map[string]*Auction

	events  event.Store
	players store.PlayerRepository
	logger  *slog.Logger
	tracer  trace.Tracer
	tp      trace.TracerProvider
	clock   clock.Clock
}

// NewManager creates a new auction Manager.
func NewManager(events event.Store, players store.PlayerRepository, logger *slog.Logger, tp trace.TracerProvider, clk clock.Clock) *Manager {
	return &Manager{
		auctions: make(map[string]*Auction),
		events:   events,
		players:  players,
		logger:   logger,
		tracer:   tp.Tracer("github.com/jensholdgaard/discord-dkp-bot/internal/auction"),
		tp:       tp,
		clock:    clk,
	}
}

// StartAuction creates and tracks a new auction.
func (m *Manager) StartAuction(ctx context.Context, itemName, startedBy string, minBid int, duration time.Duration) (*Auction, error) {
	ctx, span := m.tracer.Start(ctx, "Manager.StartAuction",
		trace.WithAttributes(
			attribute.String("item", itemName),
			attribute.String("started_by", startedBy),
		),
	)
	defer span.End()

	id := fmt.Sprintf("auction-%d", m.clock.Now().UnixNano())
	a := New(id, itemName, startedBy, minBid, duration, m.tp, m.clock)

	// Persist initial events.
	if err := m.events.Append(ctx, a.PendingEvents()...); err != nil {
		return nil, fmt.Errorf("persisting auction started events: %w", err)
	}

	m.mu.Lock()
	m.auctions[id] = a
	m.mu.Unlock()

	m.logger.InfoContext(ctx, "auction started",
		slog.String("auction_id", id),
		slog.String("item", itemName),
	)
	return a, nil
}

// PlaceBid places a bid on an active auction.
func (m *Manager) PlaceBid(ctx context.Context, auctionID, discordID string, amount int) error {
	ctx, span := m.tracer.Start(ctx, "Manager.PlaceBid",
		trace.WithAttributes(
			attribute.String("auction_id", auctionID),
			attribute.String("discord_id", discordID),
			attribute.Int("amount", amount),
		),
	)
	defer span.End()

	m.mu.RLock()
	a, ok := m.auctions[auctionID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("auction %s not found", auctionID)
	}

	// Look up the player to verify DKP.
	player, err := m.players.GetByDiscordID(ctx, discordID)
	if err != nil {
		return fmt.Errorf("player not registered: %w", err)
	}

	if err := a.PlaceBid(ctx, player.ID, amount, player.DKP); err != nil {
		return err
	}

	// Persist bid event.
	if err := m.events.Append(ctx, a.PendingEvents()...); err != nil {
		m.logger.ErrorContext(ctx, "failed to persist bid event", slog.Any("error", err))
	}

	return nil
}

// CloseAuction closes an auction and returns a result message.
func (m *Manager) CloseAuction(ctx context.Context, auctionID string) (string, error) {
	ctx, span := m.tracer.Start(ctx, "Manager.CloseAuction",
		trace.WithAttributes(attribute.String("auction_id", auctionID)),
	)
	defer span.End()

	m.mu.RLock()
	a, ok := m.auctions[auctionID]
	m.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("auction %s not found", auctionID)
	}

	winner, err := a.Close(ctx)
	if err != nil {
		return "", err
	}

	// Persist close event.
	if err := m.events.Append(ctx, a.PendingEvents()...); err != nil {
		m.logger.ErrorContext(ctx, "failed to persist close event", slog.Any("error", err))
	}

	// Clean up.
	m.mu.Lock()
	delete(m.auctions, auctionID)
	m.mu.Unlock()

	if winner == nil {
		return "", nil
	}

	return fmt.Sprintf("Auction `%s` closed! Winner: **%s** with **%d DKP**", auctionID, winner.PlayerID, winner.Amount), nil
}

// ReplayAuction reconstructs an auction from stored events.
func (m *Manager) ReplayAuction(ctx context.Context, auctionID string) (*Auction, error) {
	events, err := m.events.Load(ctx, auctionID)
	if err != nil {
		return nil, fmt.Errorf("loading events: %w", err)
	}
	return Replay(events)
}

// RecoverOpenAuctions replays all auctions from the event store and loads
// any that are still open into the in-memory map. This is used on leader
// startup to restore state after a failover.
func (m *Manager) RecoverOpenAuctions(ctx context.Context) (int, error) {
	ctx, span := m.tracer.Start(ctx, "Manager.RecoverOpenAuctions")
	defer span.End()

	// Find all auction IDs by loading all "auction.started" events.
	started, err := m.events.LoadByType(ctx, event.AuctionStarted)
	if err != nil {
		return 0, fmt.Errorf("loading auction started events: %w", err)
	}

	// Deduplicate aggregate IDs.
	seen := make(map[string]struct{}, len(started))
	var ids []string
	for _, e := range started {
		if _, ok := seen[e.AggregateID]; !ok {
			seen[e.AggregateID] = struct{}{}
			ids = append(ids, e.AggregateID)
		}
	}

	recovered := 0
	for _, id := range ids {
		a, replayErr := m.ReplayAuction(ctx, id)
		if replayErr != nil {
			m.logger.WarnContext(ctx, "failed to replay auction during recovery",
				slog.String("auction_id", id),
				slog.Any("error", replayErr),
			)
			continue
		}
		if a.Status != "open" {
			continue
		}

		m.mu.Lock()
		m.auctions[id] = a
		m.mu.Unlock()
		recovered++

		m.logger.InfoContext(ctx, "recovered open auction",
			slog.String("auction_id", id),
			slog.String("item", a.ItemName),
			slog.Int("bids", len(a.Bids)),
		)
	}

	m.logger.InfoContext(ctx, "auction recovery complete",
		slog.Int("total_started", len(ids)),
		slog.Int("recovered_open", recovered),
	)
	return recovered, nil
}

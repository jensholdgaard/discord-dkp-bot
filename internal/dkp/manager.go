package dkp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/jensholdgaard/discord-dkp-bot/internal/event"
	"github.com/jensholdgaard/discord-dkp-bot/internal/store"
)

// Manager handles DKP operations.
type Manager struct {
	players store.PlayerRepository
	events  event.Store
	logger  *slog.Logger
	tracer  trace.Tracer
}

// NewManager returns a new DKP Manager.
func NewManager(players store.PlayerRepository, events event.Store, logger *slog.Logger, tp trace.TracerProvider) *Manager {
	return &Manager{
		players: players,
		events:  events,
		logger:  logger,
		tracer:  tp.Tracer("github.com/jensholdgaard/discord-dkp-bot/internal/dkp"),
	}
}

// RegisterPlayer registers a new player character.
func (m *Manager) RegisterPlayer(ctx context.Context, discordID, characterName string) (*store.Player, error) {
	ctx, span := m.tracer.Start(ctx, "Manager.RegisterPlayer",
		trace.WithAttributes(
			attribute.String("discord_id", discordID),
			attribute.String("character_name", characterName),
		),
	)
	defer span.End()

	p := &store.Player{
		DiscordID:     discordID,
		CharacterName: characterName,
		DKP:           0,
	}
	if err := m.players.Create(ctx, p); err != nil {
		return nil, fmt.Errorf("creating player: %w", err)
	}

	data, _ := json.Marshal(event.PlayerRegisteredData{
		DiscordID:     discordID,
		CharacterName: characterName,
	})
	evt := event.Event{
		AggregateID: p.ID,
		Type:        event.PlayerRegistered,
		Data:        data,
		Version:     1,
	}
	if err := m.events.Append(ctx, evt); err != nil {
		m.logger.ErrorContext(ctx, "failed to append player registered event", slog.Any("error", err))
	}

	m.logger.InfoContext(ctx, "player registered",
		slog.String("player_id", p.ID),
		slog.String("character", characterName),
	)
	return p, nil
}

// AwardDKP adds DKP to a player.
func (m *Manager) AwardDKP(ctx context.Context, playerID string, amount int, reason string) error {
	ctx, span := m.tracer.Start(ctx, "Manager.AwardDKP",
		trace.WithAttributes(
			attribute.String("player_id", playerID),
			attribute.Int("amount", amount),
		),
	)
	defer span.End()

	if err := m.players.UpdateDKP(ctx, playerID, amount); err != nil {
		return fmt.Errorf("awarding DKP: %w", err)
	}

	data, _ := json.Marshal(event.DKPChangeData{
		PlayerID: playerID,
		Amount:   amount,
		Reason:   reason,
	})
	evt := event.Event{
		AggregateID: playerID,
		Type:        event.DKPAwarded,
		Data:        data,
		Version:     0,
	}
	if err := m.events.Append(ctx, evt); err != nil {
		m.logger.ErrorContext(ctx, "failed to append DKP awarded event", slog.Any("error", err))
	}

	m.logger.InfoContext(ctx, "DKP awarded",
		slog.String("player_id", playerID),
		slog.Int("amount", amount),
		slog.String("reason", reason),
	)
	return nil
}

// DeductDKP removes DKP from a player.
func (m *Manager) DeductDKP(ctx context.Context, playerID string, amount int, reason string) error {
	ctx, span := m.tracer.Start(ctx, "Manager.DeductDKP",
		trace.WithAttributes(
			attribute.String("player_id", playerID),
			attribute.Int("amount", amount),
		),
	)
	defer span.End()

	if err := m.players.UpdateDKP(ctx, playerID, -amount); err != nil {
		return fmt.Errorf("deducting DKP: %w", err)
	}

	data, _ := json.Marshal(event.DKPChangeData{
		PlayerID: playerID,
		Amount:   -amount,
		Reason:   reason,
	})
	evt := event.Event{
		AggregateID: playerID,
		Type:        event.DKPDeducted,
		Data:        data,
		Version:     0,
	}
	if err := m.events.Append(ctx, evt); err != nil {
		m.logger.ErrorContext(ctx, "failed to append DKP deducted event", slog.Any("error", err))
	}

	m.logger.InfoContext(ctx, "DKP deducted",
		slog.String("player_id", playerID),
		slog.Int("amount", amount),
		slog.String("reason", reason),
	)
	return nil
}

// GetPlayer returns a player by Discord ID.
func (m *Manager) GetPlayer(ctx context.Context, discordID string) (*store.Player, error) {
	ctx, span := m.tracer.Start(ctx, "Manager.GetPlayer")
	defer span.End()

	return m.players.GetByDiscordID(ctx, discordID)
}

// ListPlayers returns all players ordered by DKP.
func (m *Manager) ListPlayers(ctx context.Context) ([]store.Player, error) {
	ctx, span := m.tracer.Start(ctx, "Manager.ListPlayers")
	defer span.End()

	return m.players.List(ctx)
}

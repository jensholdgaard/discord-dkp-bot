package entstore

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jensholdgaard/discord-dkp-bot/internal/clock"
	"github.com/jensholdgaard/discord-dkp-bot/internal/store"
)

// PlayerRepo implements store.PlayerRepository using database/sql.
type PlayerRepo struct {
	db    *sql.DB
	clock clock.Clock
}

// NewPlayerRepo returns a new PlayerRepo.
func NewPlayerRepo(db *sql.DB, clk clock.Clock) *PlayerRepo {
	return &PlayerRepo{db: db, clock: clk}
}

func (r *PlayerRepo) Create(ctx context.Context, p *store.Player) error {
	now := r.clock.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now
	return r.db.QueryRowContext(ctx,
		`INSERT INTO players (discord_id, character_name, dkp, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		p.DiscordID, p.CharacterName, p.DKP, p.CreatedAt, p.UpdatedAt,
	).Scan(&p.ID)
}

func (r *PlayerRepo) GetByDiscordID(ctx context.Context, discordID string) (*store.Player, error) {
	p := &store.Player{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, discord_id, character_name, dkp, created_at, updated_at
		 FROM players WHERE discord_id = $1`, discordID,
	).Scan(&p.ID, &p.DiscordID, &p.CharacterName, &p.DKP, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("getting player by discord_id: %w", err)
	}
	return p, nil
}

func (r *PlayerRepo) GetByCharacterName(ctx context.Context, name string) (*store.Player, error) {
	p := &store.Player{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, discord_id, character_name, dkp, created_at, updated_at
		 FROM players WHERE character_name = $1`, name,
	).Scan(&p.ID, &p.DiscordID, &p.CharacterName, &p.DKP, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("getting player by character_name: %w", err)
	}
	return p, nil
}

func (r *PlayerRepo) List(ctx context.Context) ([]store.Player, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, discord_id, character_name, dkp, created_at, updated_at FROM players ORDER BY dkp DESC`)
	if err != nil {
		return nil, fmt.Errorf("listing players: %w", err)
	}
	defer rows.Close()

	var players []store.Player
	for rows.Next() {
		var p store.Player
		if err := rows.Scan(&p.ID, &p.DiscordID, &p.CharacterName, &p.DKP, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning player row: %w", err)
		}
		players = append(players, p)
	}
	return players, rows.Err()
}

func (r *PlayerRepo) UpdateDKP(ctx context.Context, id string, delta int) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE players SET dkp = dkp + $1, updated_at = $2 WHERE id = $3`,
		delta, r.clock.Now().UTC(), id,
	)
	if err != nil {
		return fmt.Errorf("updating dkp: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("player %s not found", id)
	}
	return nil
}

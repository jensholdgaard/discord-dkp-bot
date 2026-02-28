package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jensholdgaard/discord-dkp-bot/internal/store"
	"github.com/jmoiron/sqlx"
)

// PlayerRepo implements store.PlayerRepository with sqlx.
type PlayerRepo struct {
	db *sqlx.DB
}

// NewPlayerRepo returns a new PlayerRepo.
func NewPlayerRepo(db *sqlx.DB) *PlayerRepo {
	return &PlayerRepo{db: db}
}

func (r *PlayerRepo) Create(ctx context.Context, p *store.Player) error {
	query := `INSERT INTO players (discord_id, character_name, dkp, created_at, updated_at)
	           VALUES ($1, $2, $3, $4, $5)
	           RETURNING id`
	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now
	return r.db.QueryRowContext(ctx, query, p.DiscordID, p.CharacterName, p.DKP, p.CreatedAt, p.UpdatedAt).Scan(&p.ID)
}

func (r *PlayerRepo) GetByDiscordID(ctx context.Context, discordID string) (*store.Player, error) {
	var p store.Player
	err := r.db.GetContext(ctx, &p, `SELECT * FROM players WHERE discord_id = $1`, discordID)
	if err != nil {
		return nil, fmt.Errorf("getting player by discord_id: %w", err)
	}
	return &p, nil
}

func (r *PlayerRepo) GetByCharacterName(ctx context.Context, name string) (*store.Player, error) {
	var p store.Player
	err := r.db.GetContext(ctx, &p, `SELECT * FROM players WHERE character_name = $1`, name)
	if err != nil {
		return nil, fmt.Errorf("getting player by character_name: %w", err)
	}
	return &p, nil
}

func (r *PlayerRepo) List(ctx context.Context) ([]store.Player, error) {
	var players []store.Player
	err := r.db.SelectContext(ctx, &players, `SELECT * FROM players ORDER BY dkp DESC`)
	if err != nil {
		return nil, fmt.Errorf("listing players: %w", err)
	}
	return players, nil
}

func (r *PlayerRepo) UpdateDKP(ctx context.Context, id string, delta int) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE players SET dkp = dkp + $1, updated_at = $2 WHERE id = $3`,
		delta, time.Now().UTC(), id,
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

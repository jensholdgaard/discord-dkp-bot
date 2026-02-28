package store

import (
	"context"
	"time"
)

// Player represents a registered player.
type Player struct {
	ID            string    `db:"id"`
	DiscordID     string    `db:"discord_id"`
	CharacterName string    `db:"character_name"`
	DKP           int       `db:"dkp"`
	CreatedAt     time.Time `db:"created_at"`
	UpdatedAt     time.Time `db:"updated_at"`
}

// Auction represents an auction record.
type Auction struct {
	ID        string    `db:"id"`
	ItemName  string    `db:"item_name"`
	StartedBy string    `db:"started_by"`
	MinBid    int       `db:"min_bid"`
	Status    string    `db:"status"` // "open", "closed", "cancelled"
	WinnerID  *string   `db:"winner_id"`
	WinAmount *int      `db:"win_amount"`
	CreatedAt time.Time `db:"created_at"`
	ClosedAt  *time.Time `db:"closed_at"`
}

// PlayerRepository defines player persistence operations.
type PlayerRepository interface {
	Create(ctx context.Context, p *Player) error
	GetByDiscordID(ctx context.Context, discordID string) (*Player, error)
	GetByCharacterName(ctx context.Context, name string) (*Player, error)
	List(ctx context.Context) ([]Player, error)
	UpdateDKP(ctx context.Context, id string, delta int) error
}

// AuctionRepository defines auction persistence operations.
type AuctionRepository interface {
	Create(ctx context.Context, a *Auction) error
	GetByID(ctx context.Context, id string) (*Auction, error)
	Close(ctx context.Context, id string, winnerID string, amount int) error
	Cancel(ctx context.Context, id string) error
	ListOpen(ctx context.Context) ([]Auction, error)
}

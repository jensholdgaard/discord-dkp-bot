package entstore

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jensholdgaard/discord-dkp-bot/internal/clock"
	"github.com/jensholdgaard/discord-dkp-bot/internal/store"
)

// AuctionRepo implements store.AuctionRepository using database/sql.
type AuctionRepo struct {
	db    *sql.DB
	clock clock.Clock
}

// NewAuctionRepo returns a new AuctionRepo.
func NewAuctionRepo(db *sql.DB, clk clock.Clock) *AuctionRepo {
	return &AuctionRepo{db: db, clock: clk}
}

func (r *AuctionRepo) Create(ctx context.Context, a *store.Auction) error {
	a.CreatedAt = r.clock.Now().UTC()
	a.Status = "open"
	return r.db.QueryRowContext(ctx,
		`INSERT INTO auctions (item_name, started_by, min_bid, status, created_at)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		a.ItemName, a.StartedBy, a.MinBid, a.Status, a.CreatedAt,
	).Scan(&a.ID)
}

func (r *AuctionRepo) GetByID(ctx context.Context, id string) (*store.Auction, error) {
	a := &store.Auction{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, item_name, started_by, min_bid, status, winner_id, win_amount, created_at, closed_at
		 FROM auctions WHERE id = $1`, id,
	).Scan(&a.ID, &a.ItemName, &a.StartedBy, &a.MinBid, &a.Status, &a.WinnerID, &a.WinAmount, &a.CreatedAt, &a.ClosedAt)
	if err != nil {
		return nil, fmt.Errorf("getting auction: %w", err)
	}
	return a, nil
}

func (r *AuctionRepo) Close(ctx context.Context, id string, winnerID string, amount int) error {
	now := r.clock.Now().UTC()
	result, err := r.db.ExecContext(ctx,
		`UPDATE auctions SET status = 'closed', winner_id = $1, win_amount = $2, closed_at = $3
		 WHERE id = $4 AND status = 'open'`,
		winnerID, amount, now, id,
	)
	if err != nil {
		return fmt.Errorf("closing auction: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("auction %s not found or already closed", id)
	}
	return nil
}

func (r *AuctionRepo) Cancel(ctx context.Context, id string) error {
	now := r.clock.Now().UTC()
	result, err := r.db.ExecContext(ctx,
		`UPDATE auctions SET status = 'canceled', closed_at = $1 WHERE id = $2 AND status = 'open'`,
		now, id,
	)
	if err != nil {
		return fmt.Errorf("canceling auction: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("auction %s not found or already closed", id)
	}
	return nil
}

func (r *AuctionRepo) ListOpen(ctx context.Context) ([]store.Auction, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, item_name, started_by, min_bid, status, winner_id, win_amount, created_at, closed_at
		 FROM auctions WHERE status = 'open' ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("listing open auctions: %w", err)
	}
	defer rows.Close()

	var auctions []store.Auction
	for rows.Next() {
		var a store.Auction
		if err := rows.Scan(&a.ID, &a.ItemName, &a.StartedBy, &a.MinBid, &a.Status, &a.WinnerID, &a.WinAmount, &a.CreatedAt, &a.ClosedAt); err != nil {
			return nil, fmt.Errorf("scanning auction row: %w", err)
		}
		auctions = append(auctions, a)
	}
	return auctions, rows.Err()
}

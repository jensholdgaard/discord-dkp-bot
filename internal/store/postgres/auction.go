package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jensholdgaard/discord-dkp-bot/internal/store"
	"github.com/jmoiron/sqlx"
)

// AuctionRepo implements store.AuctionRepository with sqlx.
type AuctionRepo struct {
	db *sqlx.DB
}

// NewAuctionRepo returns a new AuctionRepo.
func NewAuctionRepo(db *sqlx.DB) *AuctionRepo {
	return &AuctionRepo{db: db}
}

func (r *AuctionRepo) Create(ctx context.Context, a *store.Auction) error {
	query := `INSERT INTO auctions (item_name, started_by, min_bid, status, created_at)
	           VALUES ($1, $2, $3, $4, $5) RETURNING id`
	a.CreatedAt = time.Now().UTC()
	a.Status = "open"
	return r.db.QueryRowContext(ctx, query, a.ItemName, a.StartedBy, a.MinBid, a.Status, a.CreatedAt).Scan(&a.ID)
}

func (r *AuctionRepo) GetByID(ctx context.Context, id string) (*store.Auction, error) {
	var a store.Auction
	err := r.db.GetContext(ctx, &a, `SELECT * FROM auctions WHERE id = $1`, id)
	if err != nil {
		return nil, fmt.Errorf("getting auction: %w", err)
	}
	return &a, nil
}

func (r *AuctionRepo) Close(ctx context.Context, id string, winnerID string, amount int) error {
	now := time.Now().UTC()
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
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx,
		`UPDATE auctions SET status = 'cancelled', closed_at = $1 WHERE id = $2 AND status = 'open'`,
		now, id,
	)
	if err != nil {
		return fmt.Errorf("cancelling auction: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("auction %s not found or already closed", id)
	}
	return nil
}

func (r *AuctionRepo) ListOpen(ctx context.Context) ([]store.Auction, error) {
	var auctions []store.Auction
	err := r.db.SelectContext(ctx, &auctions, `SELECT * FROM auctions WHERE status = 'open' ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("listing open auctions: %w", err)
	}
	return auctions, nil
}

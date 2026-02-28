package event

import (
	"encoding/json"
	"time"
)

// Type identifies an event kind.
type Type string

const (
	AuctionStarted  Type = "auction.started"
	AuctionBidPlaced Type = "auction.bid_placed"
	AuctionClosed    Type = "auction.closed"
	AuctionCancelled Type = "auction.cancelled"

	DKPAwarded  Type = "dkp.awarded"
	DKPDeducted Type = "dkp.deducted"
	DKPAdjusted Type = "dkp.adjusted"

	PlayerRegistered Type = "player.registered"
)

// Event represents a single domain event.
type Event struct {
	ID          string          `json:"id" db:"id"`
	AggregateID string          `json:"aggregate_id" db:"aggregate_id"`
	Type        Type            `json:"type" db:"type"`
	Data        json.RawMessage `json:"data" db:"data"`
	Version     int             `json:"version" db:"version"`
	CreatedAt   time.Time       `json:"created_at" db:"created_at"`
}

// AuctionStartedData is the payload for AuctionStarted events.
type AuctionStartedData struct {
	ItemName  string        `json:"item_name"`
	StartedBy string        `json:"started_by"`
	MinBid    int           `json:"min_bid"`
	Duration  time.Duration `json:"duration"`
}

// BidPlacedData is the payload for AuctionBidPlaced events.
type BidPlacedData struct {
	PlayerID string `json:"player_id"`
	Amount   int    `json:"amount"`
}

// AuctionClosedData is the payload for AuctionClosed events.
type AuctionClosedData struct {
	WinnerID string `json:"winner_id"`
	Amount   int    `json:"amount"`
}

// DKPChangeData is the payload for DKP events.
type DKPChangeData struct {
	PlayerID string `json:"player_id"`
	Amount   int    `json:"amount"`
	Reason   string `json:"reason"`
}

// PlayerRegisteredData is the payload for PlayerRegistered events.
type PlayerRegisteredData struct {
	DiscordID     string `json:"discord_id"`
	CharacterName string `json:"character_name"`
}

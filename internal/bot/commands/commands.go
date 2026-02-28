package commands

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/jensholdgaard/discord-dkp-bot/internal/auction"
	"github.com/jensholdgaard/discord-dkp-bot/internal/dkp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Handlers process Discord interactions.
type Handlers struct {
	dkpMgr     *dkp.Manager
	auctionMgr *auction.Manager
	logger     *slog.Logger
	tracer     trace.Tracer
}

// NewHandlers creates new command handlers.
func NewHandlers(dkpMgr *dkp.Manager, auctionMgr *auction.Manager, logger *slog.Logger, tp trace.TracerProvider) *Handlers {
	return &Handlers{
		dkpMgr:     dkpMgr,
		auctionMgr: auctionMgr,
		logger:     logger,
		tracer:     tp.Tracer("github.com/jensholdgaard/discord-dkp-bot/internal/bot/commands"),
	}
}

// SlashCommands returns the slash command definitions.
func SlashCommands() []*discordgo.ApplicationCommand {
	return []*discordgo.ApplicationCommand{
		{
			Name:        "register",
			Description: "Register your character for DKP tracking",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "character",
					Description: "Your in-game character name",
					Required:    true,
				},
			},
		},
		{
			Name:        "dkp",
			Description: "Check your DKP balance",
		},
		{
			Name:        "dkp-list",
			Description: "List all players and their DKP",
		},
		{
			Name:        "dkp-add",
			Description: "Add DKP to a player (admin only)",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionUser,
					Name:        "player",
					Description: "The player to award DKP to",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "amount",
					Description: "Amount of DKP to award",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "reason",
					Description: "Reason for the DKP award",
					Required:    true,
				},
			},
		},
		{
			Name:        "dkp-remove",
			Description: "Remove DKP from a player (admin only)",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionUser,
					Name:        "player",
					Description: "The player to deduct DKP from",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "amount",
					Description: "Amount of DKP to deduct",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "reason",
					Description: "Reason for the DKP deduction",
					Required:    true,
				},
			},
		},
		{
			Name:        "auction-start",
			Description: "Start an item auction",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "item",
					Description: "Item name to auction",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "min-bid",
					Description: "Minimum bid amount",
					Required:    false,
				},
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "duration",
					Description: "Auction duration in minutes (default: 5)",
					Required:    false,
				},
			},
		},
		{
			Name:        "bid",
			Description: "Place a bid on the current auction",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "auction-id",
					Description: "Auction ID to bid on",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "amount",
					Description: "Bid amount",
					Required:    true,
				},
			},
		},
		{
			Name:        "auction-close",
			Description: "Close an auction (admin only)",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "auction-id",
					Description: "Auction ID to close",
					Required:    true,
				},
			},
		},
	}
}

// InteractionCreate handles incoming slash command interactions.
func (h *Handlers) InteractionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx, span := h.tracer.Start(context.Background(), "InteractionCreate",
		trace.WithAttributes(attribute.String("command", i.ApplicationCommandData().Name)),
	)
	defer span.End()

	switch i.ApplicationCommandData().Name {
	case "register":
		h.handleRegister(ctx, s, i)
	case "dkp":
		h.handleDKP(ctx, s, i)
	case "dkp-list":
		h.handleDKPList(ctx, s, i)
	case "dkp-add":
		h.handleDKPAdd(ctx, s, i)
	case "dkp-remove":
		h.handleDKPRemove(ctx, s, i)
	case "auction-start":
		h.handleAuctionStart(ctx, s, i)
	case "bid":
		h.handleBid(ctx, s, i)
	case "auction-close":
		h.handleAuctionClose(ctx, s, i)
	default:
		respond(s, i, "Unknown command")
	}
}

func (h *Handlers) handleRegister(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) {
	opts := i.ApplicationCommandData().Options
	charName := opts[0].StringValue()
	discordID := i.Member.User.ID

	p, err := h.dkpMgr.RegisterPlayer(ctx, discordID, charName)
	if err != nil {
		respond(s, i, fmt.Sprintf("Failed to register: %s", err))
		return
	}
	respond(s, i, fmt.Sprintf("Registered **%s** (DKP: %d)", p.CharacterName, p.DKP))
}

func (h *Handlers) handleDKP(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) {
	discordID := i.Member.User.ID
	p, err := h.dkpMgr.GetPlayer(ctx, discordID)
	if err != nil {
		respond(s, i, "You are not registered. Use `/register` first.")
		return
	}
	respond(s, i, fmt.Sprintf("**%s** — DKP: **%d**", p.CharacterName, p.DKP))
}

func (h *Handlers) handleDKPList(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) {
	players, err := h.dkpMgr.ListPlayers(ctx)
	if err != nil {
		respond(s, i, fmt.Sprintf("Error listing players: %s", err))
		return
	}
	if len(players) == 0 {
		respond(s, i, "No players registered yet.")
		return
	}
	msg := "**DKP Standings:**\n"
	for idx, p := range players {
		msg += fmt.Sprintf("%d. %s — %d DKP\n", idx+1, p.CharacterName, p.DKP)
	}
	respond(s, i, msg)
}

func (h *Handlers) handleDKPAdd(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) {
	opts := i.ApplicationCommandData().Options
	targetUser := opts[0].UserValue(s)
	amount := int(opts[1].IntValue())
	reason := opts[2].StringValue()

	target, err := h.dkpMgr.GetPlayer(ctx, targetUser.ID)
	if err != nil {
		respond(s, i, "Target player is not registered.")
		return
	}

	if err := h.dkpMgr.AwardDKP(ctx, target.ID, amount, reason); err != nil {
		respond(s, i, fmt.Sprintf("Failed to award DKP: %s", err))
		return
	}
	respond(s, i, fmt.Sprintf("Awarded **%d DKP** to **%s** for: %s", amount, target.CharacterName, reason))
}

func (h *Handlers) handleDKPRemove(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) {
	opts := i.ApplicationCommandData().Options
	targetUser := opts[0].UserValue(s)
	amount := int(opts[1].IntValue())
	reason := opts[2].StringValue()

	target, err := h.dkpMgr.GetPlayer(ctx, targetUser.ID)
	if err != nil {
		respond(s, i, "Target player is not registered.")
		return
	}

	if err := h.dkpMgr.DeductDKP(ctx, target.ID, amount, reason); err != nil {
		respond(s, i, fmt.Sprintf("Failed to deduct DKP: %s", err))
		return
	}
	respond(s, i, fmt.Sprintf("Deducted **%d DKP** from **%s** for: %s", amount, target.CharacterName, reason))
}

func (h *Handlers) handleAuctionStart(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) {
	opts := i.ApplicationCommandData().Options
	itemName := opts[0].StringValue()

	minBid := 0
	duration := 5 * time.Minute

	for _, opt := range opts[1:] {
		switch opt.Name {
		case "min-bid":
			minBid = int(opt.IntValue())
		case "duration":
			duration = time.Duration(opt.IntValue()) * time.Minute
		}
	}

	a, err := h.auctionMgr.StartAuction(ctx, itemName, i.Member.User.ID, minBid, duration)
	if err != nil {
		respond(s, i, fmt.Sprintf("Failed to start auction: %s", err))
		return
	}
	respond(s, i, fmt.Sprintf("Auction started for **%s** (ID: `%s`, Min bid: %d, Duration: %s)", itemName, a.ID, minBid, duration))
}

func (h *Handlers) handleBid(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) {
	opts := i.ApplicationCommandData().Options
	auctionID := opts[0].StringValue()
	amount := int(opts[1].IntValue())
	discordID := i.Member.User.ID

	if err := h.auctionMgr.PlaceBid(ctx, auctionID, discordID, amount); err != nil {
		respond(s, i, fmt.Sprintf("Bid failed: %s", err))
		return
	}
	respond(s, i, fmt.Sprintf("Bid of **%d DKP** placed on auction `%s`", amount, auctionID))
}

func (h *Handlers) handleAuctionClose(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) {
	opts := i.ApplicationCommandData().Options
	auctionID := opts[0].StringValue()

	result, err := h.auctionMgr.CloseAuction(ctx, auctionID)
	if err != nil {
		respond(s, i, fmt.Sprintf("Failed to close auction: %s", err))
		return
	}
	if result == "" {
		respond(s, i, fmt.Sprintf("Auction `%s` closed with no bids.", auctionID))
	} else {
		respond(s, i, result)
	}
}

func respond(s *discordgo.Session, i *discordgo.InteractionCreate, msg string) {
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: msg,
		},
	})
}

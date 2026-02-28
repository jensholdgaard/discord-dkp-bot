package bot

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/bwmarrin/discordgo"
	"go.opentelemetry.io/otel/trace"

	"github.com/jensholdgaard/discord-dkp-bot/internal/auction"
	"github.com/jensholdgaard/discord-dkp-bot/internal/bot/commands"
	"github.com/jensholdgaard/discord-dkp-bot/internal/config"
	"github.com/jensholdgaard/discord-dkp-bot/internal/dkp"
)

// Bot wraps the Discord session and command handlers.
type Bot struct {
	session  *discordgo.Session
	cfg      config.DiscordConfig
	logger   *slog.Logger
	handlers *commands.Handlers
	cmds     []*discordgo.ApplicationCommand
}

// New creates a new Bot instance.
func New(cfg config.DiscordConfig, dkpMgr *dkp.Manager, auctionMgr *auction.Manager, logger *slog.Logger, tp trace.TracerProvider) (*Bot, error) {
	session, err := discordgo.New("Bot " + cfg.Token)
	if err != nil {
		return nil, fmt.Errorf("creating discord session: %w", err)
	}

	handlers := commands.NewHandlers(dkpMgr, auctionMgr, logger, tp)

	return &Bot{
		session:  session,
		cfg:      cfg,
		logger:   logger,
		handlers: handlers,
	}, nil
}

// Start opens the Discord connection and registers slash commands.
func (b *Bot) Start(ctx context.Context) error {
	b.session.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		b.logger.InfoContext(ctx, "bot is ready", slog.String("user", s.State.User.Username))
	})

	b.session.AddHandler(b.handlers.InteractionCreate)

	if err := b.session.Open(); err != nil {
		return fmt.Errorf("opening discord session: %w", err)
	}

	// Register slash commands.
	appCmds := commands.SlashCommands()
	registered, err := b.session.ApplicationCommandBulkOverwrite(b.session.State.User.ID, b.cfg.GuildID, appCmds)
	if err != nil {
		return fmt.Errorf("registering slash commands: %w", err)
	}
	b.cmds = registered

	b.logger.InfoContext(ctx, "slash commands registered", slog.Int("count", len(registered)))
	return nil
}

// Stop gracefully closes the Discord connection.
func (b *Bot) Stop() error {
	// Remove slash commands on shutdown (optional for dev).
	for _, cmd := range b.cmds {
		if err := b.session.ApplicationCommandDelete(b.session.State.User.ID, b.cfg.GuildID, cmd.ID); err != nil {
			b.logger.Error("failed to delete command", slog.String("command", cmd.Name), slog.Any("error", err))
		}
	}
	return b.session.Close()
}

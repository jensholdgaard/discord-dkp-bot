package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/jensholdgaard/discord-dkp-bot/internal/auction"
	"github.com/jensholdgaard/discord-dkp-bot/internal/bot"
	"github.com/jensholdgaard/discord-dkp-bot/internal/clock"
	"github.com/jensholdgaard/discord-dkp-bot/internal/config"
	"github.com/jensholdgaard/discord-dkp-bot/internal/dkp"
	"github.com/jensholdgaard/discord-dkp-bot/internal/health"
	"github.com/jensholdgaard/discord-dkp-bot/internal/store"
	"github.com/jensholdgaard/discord-dkp-bot/internal/telemetry"

	// Register store drivers so they are available via store.Open.
	_ "github.com/jensholdgaard/discord-dkp-bot/internal/store/entstore"
	_ "github.com/jensholdgaard/discord-dkp-bot/internal/store/postgres"
)

var version = "dev"

func main() {
	configPath := flag.String("config", "config.yaml", "path to configuration file")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		os.Exit(0)
	}

	if err := run(*configPath); err != nil {
		slog.Error("fatal error", slog.Any("error", err))
		os.Exit(1)
	}
}

func run(configPath string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Load configuration.
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Setup telemetry.
	tp, err := telemetry.Setup(ctx, cfg.Telemetry)
	if err != nil {
		slog.Warn("telemetry setup failed, continuing without OTEL export", slog.Any("error", err))
		tp = telemetry.NewNopProvider()
	}
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			slog.Error("telemetry shutdown error", slog.Any("error", err))
		}
	}()

	logger := tp.Logger
	clk := clock.Real{}

	// Open store using the configured driver (sqlx or ent).
	repos, err := store.Open(ctx, cfg.Database, clk)
	if err != nil {
		return fmt.Errorf("opening store (driver=%s): %w", cfg.Database.Driver, err)
	}
	defer repos.Closer.Close()

	logger.InfoContext(ctx, "connected to database", slog.String("driver", cfg.Database.Driver))

	// Initialize managers.
	dkpMgr := dkp.NewManager(repos.Players, repos.Events, logger, tp.TracerProvider)
	auctionMgr := auction.NewManager(repos.Events, repos.Players, logger, tp.TracerProvider, clk)

	// Setup health checks.
	healthHandler := health.NewHandler(clk,
		health.Checker{
			Name: "database",
			Check: repos.Ping,
		},
	)

	// Start HTTP server for health checks.
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthHandler.LivenessHandler())
	mux.HandleFunc("/readyz", healthHandler.ReadinessHandler())

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: mux,
	}

	go func() {
		logger.InfoContext(ctx, "starting health server", slog.Int("port", cfg.Server.Port))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.ErrorContext(ctx, "health server error", slog.Any("error", err))
		}
	}()

	// Start Discord bot.
	discordBot, err := bot.New(cfg.Discord, dkpMgr, auctionMgr, logger, tp.TracerProvider)
	if err != nil {
		return fmt.Errorf("creating bot: %w", err)
	}

	if err := discordBot.Start(ctx); err != nil {
		return fmt.Errorf("starting bot: %w", err)
	}

	healthHandler.SetReady(true)
	logger.InfoContext(ctx, "dkpbot is running", slog.String("version", version))

	// Wait for shutdown signal.
	<-ctx.Done()
	logger.Info("shutting down...")

	healthHandler.SetReady(false)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("http server shutdown error", slog.Any("error", err))
	}

	if err := discordBot.Stop(); err != nil {
		logger.Error("bot shutdown error", slog.Any("error", err))
	}

	logger.Info("shutdown complete")
	return nil
}

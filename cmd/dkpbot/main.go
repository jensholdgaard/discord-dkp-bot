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
	"time"

	"github.com/jensholdgaard/discord-dkp-bot/internal/auction"
	"github.com/jensholdgaard/discord-dkp-bot/internal/bot"
	"github.com/jensholdgaard/discord-dkp-bot/internal/clock"
	"github.com/jensholdgaard/discord-dkp-bot/internal/config"
	"github.com/jensholdgaard/discord-dkp-bot/internal/dkp"
	"github.com/jensholdgaard/discord-dkp-bot/internal/health"
	"github.com/jensholdgaard/discord-dkp-bot/internal/leader"
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
		if shutdownErr := tp.Shutdown(context.Background()); shutdownErr != nil {
			slog.Error("telemetry shutdown error", slog.Any("error", shutdownErr))
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
			Name:  "database",
			Check: repos.Ping,
		},
	)

	// Start HTTP server for health checks (runs on all replicas).
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthHandler.LivenessHandler())
	mux.HandleFunc("/readyz", healthHandler.ReadinessHandler())

	httpServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.InfoContext(ctx, "starting health server", slog.Int("port", cfg.Server.Port))
		if listenErr := httpServer.ListenAndServe(); listenErr != nil && listenErr != http.ErrServerClosed {
			logger.ErrorContext(ctx, "health server error", slog.Any("error", listenErr))
		}
	}()

	// startBot is the core work that only the leader should run.
	startBot := func(ctx context.Context) {
		// Recover in-flight auctions from the event store so that they
		// survive leader failover.
		if n, recoverErr := auctionMgr.RecoverOpenAuctions(ctx); recoverErr != nil {
			logger.ErrorContext(ctx, "auction recovery failed", slog.Any("error", recoverErr))
		} else if n > 0 {
			logger.InfoContext(ctx, "recovered open auctions", slog.Int("count", n))
		}

		discordBot, botErr := bot.New(cfg.Discord, dkpMgr, auctionMgr, logger, tp.TracerProvider)
		if botErr != nil {
			logger.ErrorContext(ctx, "creating bot failed", slog.Any("error", botErr))
			return
		}

		if botErr = discordBot.Start(ctx); botErr != nil {
			logger.ErrorContext(ctx, "starting bot failed", slog.Any("error", botErr))
			return
		}

		healthHandler.SetReady(true)
		logger.InfoContext(ctx, "dkpbot is running (leader)", slog.String("version", version))

		// Block until leadership is lost or process is shutting down.
		<-ctx.Done()

		healthHandler.SetReady(false)
		if stopErr := discordBot.Stop(); stopErr != nil {
			logger.Error("bot shutdown error", slog.Any("error", stopErr))
		}
	}

	if cfg.LeaderElection.Enabled {
		logger.InfoContext(ctx, "leader election enabled, waiting for leadership...")

		if leaderErr := leader.Run(ctx, cfg.LeaderElection, logger, startBot, func() {
			logger.Info("lost leadership, shutting down...")
			cancel()
		}); leaderErr != nil {
			return fmt.Errorf("leader election: %w", leaderErr)
		}
	} else {
		// No leader election — run directly.
		discordBot, botErr := bot.New(cfg.Discord, dkpMgr, auctionMgr, logger, tp.TracerProvider)
		if botErr != nil {
			return fmt.Errorf("creating bot: %w", botErr)
		}

		if botErr = discordBot.Start(ctx); botErr != nil {
			return fmt.Errorf("starting bot: %w", botErr)
		}

		healthHandler.SetReady(true)
		logger.InfoContext(ctx, "dkpbot is running", slog.String("version", version))

		// Wait for shutdown signal.
		<-ctx.Done()
		logger.Info("shutting down...")

		healthHandler.SetReady(false)

		if stopErr := discordBot.Stop(); stopErr != nil {
			logger.Error("bot shutdown error", slog.Any("error", stopErr))
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("http server shutdown error", slog.Any("error", err))
	}

	logger.Info("shutdown complete")
	return nil
}

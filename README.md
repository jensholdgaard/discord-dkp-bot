# Discord DKP Bot

A cloud-native Discord bot for managing Dragon Kill Points (DKP) in MMO guilds, built with Go following CNCF best practices.

## Features

- **DKP Management** — Award, deduct, and track DKP for guild members
- **Auction System** — Run item auctions with real-time bidding using DKP
- **Event Sourcing** — Full event history for auction replay and auditability
- **Discord Slash Commands** — Modern Discord interaction model
- **OpenTelemetry** — Traces, metrics, and logs with TraceID correlation via `slog`
- **Postgres** — Persistent storage with OTEL-instrumented queries (sqlx)
- **Health Checks** — Kubernetes-ready liveness (`/healthz`) and readiness (`/readyz`) endpoints
- **Helm Chart** — Production-ready Kubernetes deployment

## Architecture

```
cmd/dkpbot/          — Single binary entry point
internal/
  config/            — YAML configuration loader
  telemetry/         — OpenTelemetry setup (traces, metrics, logs)
  health/            — Liveness and readiness HTTP handlers
  clock/             — Testable time abstraction
  event/             — Event sourcing types and store interface
  auction/           — Auction aggregate with concurrency model
  dkp/               — DKP business logic manager
  store/             — Repository interfaces
    postgres/        — Postgres implementations + migrations
  bot/               — Discord bot lifecycle
    commands/        — Slash command handlers
deploy/
  helm/dkpbot/       — Helm chart
  docker-compose.dev.yml — Local development environment
```

## Quick Start

### Prerequisites

- Go 1.23+
- Docker & Docker Compose
- PostgreSQL 16+

### Setup

```bash
# Install development tools
make setup

# Start local Postgres and OTEL collector
make dev

# Apply database migrations
make migrate

# Copy and edit config
cp config.example.yaml config.yaml
# Edit config.yaml with your Discord bot token

# Run the bot
make run
```

### Configuration

The bot takes a single `--config` flag pointing to a YAML file:

```bash
dkpbot --config /path/to/config.yaml
```

See [config.example.yaml](config.example.yaml) for all available options.

## Development

```bash
# Run tests
make test

# Run tests with coverage
make test-cover

# Lint
make lint

# Format code
make fmt

# Build binary
make build

# Build Docker image
make docker
```

## Discord Commands

| Command | Description |
|---------|-------------|
| `/register <character>` | Register your character for DKP tracking |
| `/dkp` | Check your DKP balance |
| `/dkp-list` | List all players and their DKP |
| `/dkp-add <player> <amount> <reason>` | Add DKP to a player (admin) |
| `/dkp-remove <player> <amount> <reason>` | Remove DKP from a player (admin) |
| `/auction-start <item> [min-bid] [duration]` | Start an item auction |
| `/bid <auction-id> <amount>` | Place a bid on an auction |
| `/auction-close <auction-id>` | Close an auction (admin) |

## Deployment

### Helm

```bash
helm install dkpbot deploy/helm/dkpbot \
  --set config.discord.token=YOUR_TOKEN \
  --set config.discord.guild_id=YOUR_GUILD_ID
```

### GoReleaser

```bash
# Snapshot build (no publish)
make release-snapshot

# Tagged release (CI handles this automatically)
git tag v0.1.0
git push --tags
```

## Testing

Tests follow Go's table-driven test pattern:

```bash
make test
```

## License

ISC


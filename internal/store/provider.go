package store

import (
	"context"
	"fmt"
	"io"

	"github.com/jensholdgaard/discord-dkp-bot/internal/clock"
	"github.com/jensholdgaard/discord-dkp-bot/internal/config"
	"github.com/jensholdgaard/discord-dkp-bot/internal/event"
)

// Repositories groups all repository implementations returned by a store driver.
type Repositories struct {
	Players  PlayerRepository
	Auctions AuctionRepository
	Events   event.Store
	// Closer is called to release underlying resources (e.g. DB connection).
	Closer io.Closer
	// Ping checks the underlying connection health.
	Ping func(ctx context.Context) error
}

// Driver is a function that opens a connection and returns Repositories.
type Driver func(ctx context.Context, cfg config.DatabaseConfig, clk clock.Clock) (*Repositories, error)

// registry maps driver names to their factory functions.
var registry = map[string]Driver{}

// Register adds a named driver to the global registry.
// It is intended to be called from init() in each driver package.
func Register(name string, d Driver) {
	registry[name] = d
}

// Open selects the driver specified in cfg.Driver and returns Repositories.
func Open(ctx context.Context, cfg config.DatabaseConfig, clk clock.Clock) (*Repositories, error) {
	d, ok := registry[cfg.Driver]
	if !ok {
		return nil, fmt.Errorf("unknown store driver %q (registered: %v)", cfg.Driver, registeredNames())
	}
	return d(ctx, cfg, clk)
}

func registeredNames() []string {
	names := make([]string, 0, len(registry))
	for k := range registry {
		names = append(names, k)
	}
	return names
}

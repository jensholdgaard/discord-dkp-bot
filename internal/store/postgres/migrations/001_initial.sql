-- 001_initial.sql: Initial schema for DKP bot.

CREATE TABLE IF NOT EXISTS players (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    discord_id    TEXT NOT NULL UNIQUE,
    character_name TEXT NOT NULL,
    dkp           INTEGER NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS auctions (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    item_name  TEXT NOT NULL,
    started_by TEXT NOT NULL,
    min_bid    INTEGER NOT NULL DEFAULT 0,
    status     TEXT NOT NULL DEFAULT 'open',
    winner_id  UUID REFERENCES players(id),
    win_amount INTEGER,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    closed_at  TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS events (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    aggregate_id  TEXT NOT NULL,
    type          TEXT NOT NULL,
    data          JSONB NOT NULL DEFAULT '{}',
    version       INTEGER NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_events_aggregate_id ON events(aggregate_id);
CREATE INDEX idx_events_type ON events(type);
CREATE UNIQUE INDEX idx_events_aggregate_version ON events(aggregate_id, version);

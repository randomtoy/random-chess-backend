-- +goose Up

-- Persisted move records
CREATE TABLE moves (
    id         UUID        PRIMARY KEY,
    game_id    UUID        NOT NULL REFERENCES games(id),
    ply        INT         NOT NULL,
    uci        TEXT        NOT NULL,
    from_sq    TEXT        NOT NULL,
    to_sq      TEXT        NOT NULL,
    promotion  TEXT,
    client_id  UUID        NOT NULL,
    fen_before TEXT        NOT NULL,
    fen_after  TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_moves_game_ply ON moves (game_id, ply);

-- Client participation: one move per game per client
CREATE TABLE game_players (
    game_id    UUID        NOT NULL REFERENCES games(id),
    client_id  UUID        NOT NULL,
    has_moved  BOOLEAN     NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (game_id, client_id)
);

CREATE INDEX idx_game_players_client ON game_players (client_id);

-- +goose Down
DROP TABLE game_players;
DROP INDEX IF EXISTS idx_moves_game_ply;
DROP TABLE moves;

-- +goose Up
CREATE TABLE games (
    id            UUID PRIMARY KEY,
    status        TEXT NOT NULL,
    result        TEXT,
    fen           TEXT NOT NULL,
    side_to_move  TEXT NOT NULL,
    ply_count     INT  NOT NULL DEFAULT 0,
    last_move_uci TEXT,
    last_move_at  TIMESTAMPTZ,
    state_version INT  NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_games_status ON games (status);

-- +goose Down
DROP TABLE games;

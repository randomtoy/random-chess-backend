package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/randomtoy/random-chess-backend/internal/domain/game"
	"github.com/randomtoy/random-chess-backend/internal/ports"
)

const queryGetByID = `
SELECT id, status, result, fen, side_to_move, ply_count,
       last_move_uci, last_move_at, state_version, created_at, updated_at
FROM games
WHERE id = $1`

const queryListOngoing = `
SELECT id, status, result, fen, side_to_move, ply_count,
       last_move_uci, last_move_at, state_version, created_at, updated_at
FROM games
WHERE status = 'ongoing'`

const querySaveIfVersion = `
UPDATE games SET
    status        = $1,
    result        = $2,
    fen           = $3,
    side_to_move  = $4,
    ply_count     = $5,
    last_move_uci = $6,
    last_move_at  = $7,
    state_version = $8,
    updated_at    = $9
WHERE id = $10 AND state_version = $11`

const queryInsert = `
INSERT INTO games
    (id, status, result, fen, side_to_move, ply_count,
     last_move_uci, last_move_at, state_version, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
ON CONFLICT (id) DO NOTHING`

// Store is a PostgreSQL-backed GameStore.
type Store struct {
	pool *pgxpool.Pool
}

// New creates a Store backed by the given connection pool.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) GetByID(ctx context.Context, id uuid.UUID) (*game.Game, error) {
	row := s.pool.QueryRow(ctx, queryGetByID, id)
	g, err := scanGame(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ports.ErrNotFound
	}
	return g, err
}

func (s *Store) ListOngoing(ctx context.Context) ([]*game.Game, error) {
	rows, err := s.pool.Query(ctx, queryListOngoing)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*game.Game
	for rows.Next() {
		g, err := scanGame(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// SaveIfVersion atomically updates the game only when the stored state_version
// matches expectedVersion. Returns ErrVersionConflict when the version differs.
func (s *Store) SaveIfVersion(ctx context.Context, g *game.Game, expectedVersion int) error {
	var resultStr *string
	if g.Result != nil {
		r := string(*g.Result)
		resultStr = &r
	}

	tag, err := s.pool.Exec(ctx, querySaveIfVersion,
		string(g.Status),
		resultStr,
		g.FEN,
		g.SideToMove,
		g.PlyCount,
		g.LastMoveUCI,
		g.LastMoveAt,
		g.StateVersion,
		g.UpdatedAt,
		g.ID,
		expectedVersion,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ports.ErrVersionConflict
	}
	return nil
}

// Insert persists a new game. Silently ignores duplicate IDs (ON CONFLICT DO NOTHING).
func (s *Store) Insert(ctx context.Context, g *game.Game) error {
	var resultStr *string
	if g.Result != nil {
		r := string(*g.Result)
		resultStr = &r
	}

	_, err := s.pool.Exec(ctx, queryInsert,
		g.ID,
		string(g.Status),
		resultStr,
		g.FEN,
		g.SideToMove,
		g.PlyCount,
		g.LastMoveUCI,
		g.LastMoveAt,
		g.StateVersion,
		g.CreatedAt,
		g.UpdatedAt,
	)
	return err
}

// scanGame reads a game row from either a pgx.Row or pgx.Rows.
func scanGame(s interface {
	Scan(dest ...any) error
}) (*game.Game, error) {
	var (
		id           uuid.UUID
		statusStr    string
		resultStr    *string
		fen          string
		sideToMove   string
		plyCount     int
		lastMoveUCI  *string
		lastMoveAt   *time.Time
		stateVersion int
		createdAt    time.Time
		updatedAt    time.Time
	)

	err := s.Scan(
		&id, &statusStr, &resultStr, &fen, &sideToMove, &plyCount,
		&lastMoveUCI, &lastMoveAt, &stateVersion, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}

	g := &game.Game{
		ID:           id,
		Status:       game.Status(statusStr),
		FEN:          fen,
		SideToMove:   sideToMove,
		PlyCount:     plyCount,
		LastMoveUCI:  lastMoveUCI,
		LastMoveAt:   lastMoveAt,
		StateVersion: stateVersion,
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
	}
	if resultStr != nil {
		r := game.Result(*resultStr)
		g.Result = &r
	}
	return g, nil
}

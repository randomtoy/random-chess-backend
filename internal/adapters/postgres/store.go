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

const initialFEN = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1"

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

const queryHasActive = `SELECT EXISTS(SELECT 1 FROM games WHERE status IN ('waiting','ongoing'))`

const queryClaimNextGame = `
SELECT id, status, result, fen, side_to_move, ply_count,
       last_move_uci, last_move_at, state_version, created_at, updated_at
FROM games
WHERE status IN ('waiting', 'ongoing')
  AND NOT EXISTS (
      SELECT 1 FROM game_players
      WHERE game_id = games.id AND client_id = $1
  )
ORDER BY created_at ASC
LIMIT 1
FOR UPDATE SKIP LOCKED`

const queryInsertGamePlayer = `
INSERT INTO game_players (game_id, client_id, has_moved, created_at)
VALUES ($1, $2, false, NOW())
ON CONFLICT (game_id, client_id) DO NOTHING`

const queryActivateGame = `
UPDATE games SET status = 'ongoing', updated_at = NOW()
WHERE id = $1 AND status = 'waiting'`

const queryMoveHistory = `
SELECT ply, uci, from_sq, to_sq, promotion, client_id, fen_before, fen_after, created_at
FROM moves
WHERE game_id = $1
ORDER BY ply ASC`

const queryGetGamePlayer = `
SELECT has_moved FROM game_players
WHERE game_id = $1 AND client_id = $2
FOR UPDATE`

const queryInsertMove = `
INSERT INTO moves (id, game_id, ply, uci, from_sq, to_sq, promotion, client_id, fen_before, fen_after, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`

const queryUpdateGame = `
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

const queryMarkMoved = `
UPDATE game_players SET has_moved = true
WHERE game_id = $1 AND client_id = $2`

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

func (s *Store) HasActiveGames(ctx context.Context) (bool, error) {
	var exists bool
	if err := s.pool.QueryRow(ctx, queryHasActive).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (s *Store) CreateWaitingBatch(ctx context.Context, count int) error {
	now := time.Now()
	batch := &pgx.Batch{}
	for i := 0; i < count; i++ {
		id := uuid.New()
		batch.Queue(queryInsert,
			id,
			string(game.StatusWaiting),
			nil,        // result
			initialFEN,
			"white",
			0,          // ply_count
			nil,        // last_move_uci
			nil,        // last_move_at
			0,          // state_version
			now,
			now,
		)
	}
	br := s.pool.SendBatch(ctx, batch)
	defer br.Close()
	for i := 0; i < count; i++ {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

// ClaimNextGame finds a suitable game, atomically claims it for the client, and
// transitions it from waiting to ongoing if needed.
func (s *Store) ClaimNextGame(ctx context.Context, clientID uuid.UUID) (*game.Game, []game.MoveHistoryItem, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	row := tx.QueryRow(ctx, queryClaimNextGame, clientID)
	g, err := scanGame(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, ports.ErrNoGamesAvailable
	}
	if err != nil {
		return nil, nil, err
	}

	// Insert game_players row.
	tag, err := tx.Exec(ctx, queryInsertGamePlayer, g.ID, clientID)
	if err != nil {
		return nil, nil, err
	}
	// If 0 rows inserted (concurrent duplicate), bail — usecase will retry after batch creation.
	if tag.RowsAffected() == 0 {
		return nil, nil, ports.ErrNoGamesAvailable
	}

	// Transition waiting -> ongoing (no-op if already ongoing).
	if _, err := tx.Exec(ctx, queryActivateGame, g.ID); err != nil {
		return nil, nil, err
	}
	if g.Status == game.StatusWaiting {
		g.Status = game.StatusOngoing
	}

	history, err := fetchMoveHistory(ctx, tx, g.ID)
	if err != nil {
		return nil, nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, nil, err
	}
	return g, history, nil
}

func (s *Store) GetGameWithHistory(ctx context.Context, id uuid.UUID) (*game.Game, []game.MoveHistoryItem, error) {
	g, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	hist, err := fetchMoveHistory(ctx, s.pool, id)
	if err != nil {
		return nil, nil, err
	}
	return g, hist, nil
}

// PersistMove atomically checks the client assignment, inserts the move, updates
// the game state (with version CAS), marks the player as moved, and returns the
// full ordered move history.
func (s *Store) PersistMove(
	ctx context.Context,
	gameID, clientID uuid.UUID,
	newGame *game.Game,
	rec game.MoveRecord,
	ply int,
) ([]game.MoveHistoryItem, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Lock and check game_players row.
	var hasMoved bool
	err = tx.QueryRow(ctx, queryGetGamePlayer, gameID, clientID).Scan(&hasMoved)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ports.ErrNotAssigned
	}
	if err != nil {
		return nil, err
	}
	if hasMoved {
		return nil, ports.ErrAlreadyMoved
	}

	// Insert move record.
	fromSq := rec.UCI[:2]
	toSq := rec.UCI[2:4]
	var promotion *string
	if len(rec.UCI) == 5 {
		p := rec.UCI[4:]
		promotion = &p
	}
	if _, err := tx.Exec(ctx, queryInsertMove,
		rec.ID, gameID, ply, rec.UCI, fromSq, toSq, promotion,
		clientID, rec.FENBefore, rec.FENAfter, rec.CreatedAt,
	); err != nil {
		return nil, err
	}

	// Update game (CAS on state_version).
	var resultStr *string
	if newGame.Result != nil {
		r := string(*newGame.Result)
		resultStr = &r
	}
	expectedVersion := newGame.StateVersion - 1
	tag, err := tx.Exec(ctx, queryUpdateGame,
		string(newGame.Status), resultStr, newGame.FEN, newGame.SideToMove,
		newGame.PlyCount, newGame.LastMoveUCI, newGame.LastMoveAt,
		newGame.StateVersion, newGame.UpdatedAt,
		gameID, expectedVersion,
	)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ports.ErrVersionConflict
	}

	// Mark player as moved.
	if _, err := tx.Exec(ctx, queryMarkMoved, gameID, clientID); err != nil {
		return nil, err
	}

	// Return full history.
	history, err := fetchMoveHistory(ctx, tx, gameID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return history, nil
}

// fetchMoveHistory queries moves for gameID using any pgx querier (pool or tx).
func fetchMoveHistory(ctx context.Context, q interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}, gameID uuid.UUID) ([]game.MoveHistoryItem, error) {
	rows, err := q.Query(ctx, queryMoveHistory, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []game.MoveHistoryItem{}
	for rows.Next() {
		var item game.MoveHistoryItem
		var clientID uuid.UUID
		if err := rows.Scan(
			&item.Ply, &item.UCI, &item.FromSq, &item.ToSq, &item.Promotion,
			&clientID, &item.FENBefore, &item.FENAfter, &item.CreatedAt,
		); err != nil {
			return nil, err
		}
		item.ClientID = clientID
		out = append(out, item)
	}
	return out, rows.Err()
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

package ports

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/randomtoy/random-chess-backend/internal/domain/game"
)

// Sentinel store errors.
var (
	ErrNotFound         = errors.New("not found")
	ErrVersionConflict  = errors.New("version conflict")
	ErrNoGamesAvailable = errors.New("no games available")
	ErrAlreadyMoved     = errors.New("already moved in this game")
	ErrNotAssigned      = errors.New("not assigned to this game")
)

// GameStore is the persistence interface for games.
type GameStore interface {
	GetByID(ctx context.Context, id uuid.UUID) (*game.Game, error)
	ListOngoing(ctx context.Context) ([]*game.Game, error)
	// SaveIfVersion overwrites the game only when the stored StateVersion
	// equals expectedVersion. Returns ErrVersionConflict otherwise.
	SaveIfVersion(ctx context.Context, g *game.Game, expectedVersion int) error

	// HasActiveGames returns true if any game is in waiting or ongoing status.
	HasActiveGames(ctx context.Context) (bool, error)

	// CreateWaitingBatch inserts count new games in 'waiting' status.
	CreateWaitingBatch(ctx context.Context, count int) error

	// ClaimNextGame finds a game in waiting/ongoing status that clientID has not
	// played, atomically inserts a game_players row, and returns the game with its
	// current move history. Returns ErrNoGamesAvailable if nothing is found.
	ClaimNextGame(ctx context.Context, clientID uuid.UUID) (*game.Game, []game.MoveHistoryItem, error)

	// GetGameWithHistory returns a game and its ordered move history.
	GetGameWithHistory(ctx context.Context, id uuid.UUID) (*game.Game, []game.MoveHistoryItem, error)

	// PersistMove atomically verifies that clientID is assigned and has not moved,
	// inserts the move record, updates the game row (CAS on state_version), marks
	// the player as moved, and returns the full ordered move history.
	// Returns ErrNotAssigned, ErrAlreadyMoved, or ErrVersionConflict on failure.
	PersistMove(
		ctx context.Context,
		gameID, clientID uuid.UUID,
		newGame *game.Game,
		rec game.MoveRecord,
		ply int,
	) ([]game.MoveHistoryItem, error)
}

// RateLimiter gates requests by IP and optional client token.
type RateLimiter interface {
	Allow(ip, token string) bool
}

package ports

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/randomtoy/random-chess-backend/internal/domain/game"
)

// Sentinel store errors.
var (
	ErrNotFound        = errors.New("not found")
	ErrVersionConflict = errors.New("version conflict")
)

// GameStore is the persistence interface for games.
type GameStore interface {
	GetByID(ctx context.Context, id uuid.UUID) (*game.Game, error)
	ListOngoing(ctx context.Context) ([]*game.Game, error)
	// SaveIfVersion overwrites the game only when the stored StateVersion
	// equals expectedVersion. Returns ErrVersionConflict otherwise.
	SaveIfVersion(ctx context.Context, g *game.Game, expectedVersion int) error
}

// RateLimiter gates requests by IP and optional client token.
type RateLimiter interface {
	Allow(ip, token string) bool
}

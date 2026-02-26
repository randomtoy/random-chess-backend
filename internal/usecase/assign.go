package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/randomtoy/random-chess-backend/internal/domain/game"
	"github.com/randomtoy/random-chess-backend/internal/ports"
)

// AssignResult is the value returned by Assign.
type AssignResult struct {
	Game         *game.Game
	AssignmentID uuid.UUID
	AssignedAt   time.Time
}

// Assigner handles game assignment.
type Assigner struct {
	store ports.GameStore
	rl    ports.RateLimiter
}

func NewAssigner(store ports.GameStore, rl ports.RateLimiter) *Assigner {
	return &Assigner{store: store, rl: rl}
}

var ErrRateLimited = errors.New("rate limited")
var ErrNoGamesAvailable = errors.New("no ongoing games available")

func (a *Assigner) Assign(ctx context.Context, ip, token string) (AssignResult, error) {
	if !a.rl.Allow(ip, token) {
		return AssignResult{}, ErrRateLimited
	}
	games, err := a.store.ListOngoing(ctx)
	if err != nil {
		return AssignResult{}, err
	}
	if len(games) == 0 {
		return AssignResult{}, ErrNoGamesAvailable
	}
	now := time.Now()
	return AssignResult{
		Game:         games[0],
		AssignmentID: uuid.New(),
		AssignedAt:   now,
	}, nil
}

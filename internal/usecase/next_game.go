package usecase

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/randomtoy/random-chess-backend/internal/domain/game"
	"github.com/randomtoy/random-chess-backend/internal/ports"
)

// NextGameResult is the value returned by NextGame.GetNext.
type NextGameResult struct {
	Game    *game.Game
	History []game.MoveHistoryItem
}

// NextGame handles matchmaking: find (or create) a game for an anonymous client.
type NextGame struct {
	store     ports.GameStore
	rl        ports.RateLimiter
	batchSize int
}

func NewNextGame(store ports.GameStore, rl ports.RateLimiter, batchSize int) *NextGame {
	return &NextGame{store: store, rl: rl, batchSize: batchSize}
}

// GetNext returns a game that clientID has not played before.
// If no suitable game exists, a batch of waiting games is created and the
// search is retried once. Returns ErrNoGamesAvailable if still nothing found.
func (n *NextGame) GetNext(ctx context.Context, ip, token string, clientID uuid.UUID) (NextGameResult, error) {
	if !n.rl.Allow(ip, token) {
		return NextGameResult{}, ErrRateLimited
	}

	g, hist, err := n.store.ClaimNextGame(ctx, clientID)
	if err == nil {
		return NextGameResult{Game: g, History: hist}, nil
	}
	if !errors.Is(err, ports.ErrNoGamesAvailable) {
		return NextGameResult{}, err
	}

	// No suitable game found — create a batch and retry once.
	if createErr := n.store.CreateWaitingBatch(ctx, n.batchSize); createErr != nil {
		return NextGameResult{}, createErr
	}

	g, hist, err = n.store.ClaimNextGame(ctx, clientID)
	if err != nil {
		return NextGameResult{}, err
	}
	return NextGameResult{Game: g, History: hist}, nil
}

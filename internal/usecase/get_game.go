package usecase

import (
	"context"

	"github.com/google/uuid"

	"github.com/randomtoy/random-chess-backend/internal/domain/game"
	"github.com/randomtoy/random-chess-backend/internal/ports"
)

// GameGetter handles single-game retrieval.
type GameGetter struct {
	store ports.GameStore
	rl    ports.RateLimiter
}

func NewGameGetter(store ports.GameStore, rl ports.RateLimiter) *GameGetter {
	return &GameGetter{store: store, rl: rl}
}

func (g *GameGetter) GetGame(ctx context.Context, ip, token string, id uuid.UUID) (*game.Game, error) {
	if !g.rl.Allow(ip, token) {
		return nil, ErrRateLimited
	}
	return g.store.GetByID(ctx, id)
}

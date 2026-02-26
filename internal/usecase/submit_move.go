package usecase

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/randomtoy/random-chess-backend/internal/domain/game"
	"github.com/randomtoy/random-chess-backend/internal/ports"
)

// SubmitMoveRequest is the input to SubmitMove.
type SubmitMoveRequest struct {
	UCI             string
	ExpectedVersion int
	ClientNonce     *string
}

// SubmitMoveResult is the output of a successful SubmitMove.
type SubmitMoveResult struct {
	Move            game.MoveRecord
	Game            *game.Game
	ShouldFetchNext bool
}

// MoveSubmitter handles move submission.
type MoveSubmitter struct {
	store ports.GameStore
	rl    ports.RateLimiter
}

func NewMoveSubmitter(store ports.GameStore, rl ports.RateLimiter) *MoveSubmitter {
	return &MoveSubmitter{store: store, rl: rl}
}

func (m *MoveSubmitter) SubmitMove(ctx context.Context, ip, token string, gameID uuid.UUID, req SubmitMoveRequest) (SubmitMoveResult, error) {
	if !m.rl.Allow(ip, token) {
		return SubmitMoveResult{}, ErrRateLimited
	}

	g, err := m.store.GetByID(ctx, gameID)
	if err != nil {
		return SubmitMoveResult{}, err
	}

	if g.StateVersion != req.ExpectedVersion {
		return SubmitMoveResult{}, ports.ErrVersionConflict
	}

	versionBeforeMove := g.StateVersion

	newGame, rec, err := g.ApplyMove(req.UCI, time.Now())
	if err != nil {
		return SubmitMoveResult{}, err
	}

	if err := m.store.SaveIfVersion(ctx, newGame, versionBeforeMove); err != nil {
		return SubmitMoveResult{}, err
	}

	return SubmitMoveResult{Move: rec, Game: newGame, ShouldFetchNext: newGame.Status != game.StatusOngoing}, nil
}

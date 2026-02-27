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
	History         []game.MoveHistoryItem
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

// SubmitMove validates and applies a move for clientID in gameID.
// clientID must have been assigned to the game via GetNext and must not have
// already moved. Returns ErrNotAssigned (403), ErrAlreadyMoved (409),
// ErrVersionConflict (409), or domain errors on invalid/illegal moves (422).
func (m *MoveSubmitter) SubmitMove(
	ctx context.Context,
	ip, token string,
	gameID, clientID uuid.UUID,
	req SubmitMoveRequest,
) (SubmitMoveResult, error) {
	if !m.rl.Allow(ip, token) {
		return SubmitMoveResult{}, ErrRateLimited
	}

	// Load current game state for domain validation.
	g, err := m.store.GetByID(ctx, gameID)
	if err != nil {
		return SubmitMoveResult{}, err
	}

	// Client-side version check (early fast-fail before taking locks).
	if g.StateVersion != req.ExpectedVersion {
		return SubmitMoveResult{}, ports.ErrVersionConflict
	}

	// Apply domain move (pure, no side effects).
	newGame, rec, err := g.ApplyMove(req.UCI, time.Now())
	if err != nil {
		return SubmitMoveResult{}, err
	}

	// ply is 0-indexed: newGame.PlyCount is already incremented.
	ply := newGame.PlyCount - 1

	// Atomically persist: checks assignment, has_moved, CAS on version.
	history, err := m.store.PersistMove(ctx, gameID, clientID, newGame, rec, ply)
	if err != nil {
		return SubmitMoveResult{}, err
	}

	return SubmitMoveResult{
		Move:            rec,
		Game:            newGame,
		History:         history,
		ShouldFetchNext: newGame.Status != game.StatusOngoing,
	}, nil
}

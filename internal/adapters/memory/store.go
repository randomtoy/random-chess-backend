package memory

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/randomtoy/random-chess-backend/internal/domain/game"
	"github.com/randomtoy/random-chess-backend/internal/ports"
)

// Store is a thread-safe in-memory GameStore.
type Store struct {
	mu    sync.RWMutex
	games map[uuid.UUID]*game.Game
}

// New creates a Store pre-seeded with seedCount games from the initial position.
func New(seedCount int) *Store {
	s := &Store{games: make(map[uuid.UUID]*game.Game, seedCount)}
	now := time.Now()
	for i := 0; i < seedCount; i++ {
		g := game.NewGame(uuid.New(), now)
		s.games[g.ID] = g
	}
	return s
}

func (s *Store) GetByID(_ context.Context, id uuid.UUID) (*game.Game, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	g, ok := s.games[id]
	if !ok {
		return nil, ports.ErrNotFound
	}
	return g, nil
}

func (s *Store) ListOngoing(_ context.Context) ([]*game.Game, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*game.Game
	for _, g := range s.games {
		if g.Status == game.StatusOngoing {
			out = append(out, g)
		}
	}
	return out, nil
}

// SaveIfVersion overwrites the game only when the current stored StateVersion
// equals expectedVersion, providing optimistic concurrency safety.
func (s *Store) SaveIfVersion(_ context.Context, g *game.Game, expectedVersion int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.games[g.ID]
	if !ok {
		return ports.ErrNotFound
	}
	if cur.StateVersion != expectedVersion {
		return ports.ErrVersionConflict
	}
	s.games[g.ID] = g
	return nil
}

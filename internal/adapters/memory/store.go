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
	mu sync.Mutex

	games map[uuid.UUID]*game.Game

	// assigned: gameID -> set of clientIDs that have been assigned
	assigned map[uuid.UUID]map[uuid.UUID]struct{}

	// moved: gameID -> set of clientIDs that have already made their move
	moved map[uuid.UUID]map[uuid.UUID]struct{}

	// history: gameID -> ordered move history
	history map[uuid.UUID][]game.MoveHistoryItem
}

// New creates a Store pre-seeded with seedCount games from the initial position.
func New(seedCount int) *Store {
	s := &Store{
		games:    make(map[uuid.UUID]*game.Game, seedCount),
		assigned: make(map[uuid.UUID]map[uuid.UUID]struct{}),
		moved:    make(map[uuid.UUID]map[uuid.UUID]struct{}),
		history:  make(map[uuid.UUID][]game.MoveHistoryItem),
	}
	now := time.Now()
	for i := 0; i < seedCount; i++ {
		g := game.NewGame(uuid.New(), now)
		s.games[g.ID] = g
	}
	return s
}

func (s *Store) GetByID(_ context.Context, id uuid.UUID) (*game.Game, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.games[id]
	if !ok {
		return nil, ports.ErrNotFound
	}
	return g, nil
}

func (s *Store) ListOngoing(_ context.Context) ([]*game.Game, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
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

func (s *Store) HasActiveGames(_ context.Context) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, g := range s.games {
		if g.Status == game.StatusWaiting || g.Status == game.StatusOngoing {
			return true, nil
		}
	}
	return false, nil
}

func (s *Store) CreateWaitingBatch(_ context.Context, count int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for i := 0; i < count; i++ {
		id := uuid.New()
		g := game.NewGame(id, now)
		// NewGame sets StatusOngoing; override to StatusWaiting.
		waiting := *g
		waiting.Status = game.StatusWaiting
		s.games[id] = &waiting
	}
	return nil
}

func (s *Store) ClaimNextGame(_ context.Context, clientID uuid.UUID) (*game.Game, []game.MoveHistoryItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var chosen *game.Game
	for _, g := range s.games {
		if g.Status != game.StatusWaiting && g.Status != game.StatusOngoing {
			continue
		}
		if assignedSet, ok := s.assigned[g.ID]; ok {
			if _, alreadyAssigned := assignedSet[clientID]; alreadyAssigned {
				continue
			}
		}
		chosen = g
		break
	}
	if chosen == nil {
		return nil, nil, ports.ErrNoGamesAvailable
	}

	// Claim.
	if s.assigned[chosen.ID] == nil {
		s.assigned[chosen.ID] = make(map[uuid.UUID]struct{})
	}
	s.assigned[chosen.ID][clientID] = struct{}{}

	// Transition waiting -> ongoing.
	if chosen.Status == game.StatusWaiting {
		updated := *chosen
		updated.Status = game.StatusOngoing
		s.games[chosen.ID] = &updated
		chosen = &updated
	}

	hist := s.history[chosen.ID]
	if hist == nil {
		hist = []game.MoveHistoryItem{}
	}
	return chosen, hist, nil
}

func (s *Store) GetGameWithHistory(_ context.Context, id uuid.UUID) (*game.Game, []game.MoveHistoryItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.games[id]
	if !ok {
		return nil, nil, ports.ErrNotFound
	}
	hist := s.history[id]
	if hist == nil {
		hist = []game.MoveHistoryItem{}
	}
	return g, hist, nil
}

func (s *Store) PersistMove(
	_ context.Context,
	gameID, clientID uuid.UUID,
	newGame *game.Game,
	rec game.MoveRecord,
	ply int,
) ([]game.MoveHistoryItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check assignment.
	assignedSet, ok := s.assigned[gameID]
	if !ok {
		return nil, ports.ErrNotAssigned
	}
	if _, assigned := assignedSet[clientID]; !assigned {
		return nil, ports.ErrNotAssigned
	}

	// Check has_moved.
	if movedSet, ok := s.moved[gameID]; ok {
		if _, moved := movedSet[clientID]; moved {
			return nil, ports.ErrAlreadyMoved
		}
	}

	// CAS check.
	cur, ok := s.games[gameID]
	if !ok {
		return nil, ports.ErrNotFound
	}
	if cur.StateVersion != newGame.StateVersion-1 {
		return nil, ports.ErrVersionConflict
	}

	// Persist.
	s.games[gameID] = newGame

	if s.moved[gameID] == nil {
		s.moved[gameID] = make(map[uuid.UUID]struct{})
	}
	s.moved[gameID][clientID] = struct{}{}

	fromSq := rec.UCI[:2]
	toSq := rec.UCI[2:4]
	var promotion *string
	if len(rec.UCI) == 5 {
		p := rec.UCI[4:]
		promotion = &p
	}
	item := game.MoveHistoryItem{
		Ply:       ply,
		UCI:       rec.UCI,
		FromSq:    fromSq,
		ToSq:      toSq,
		Promotion: promotion,
		ClientID:  clientID,
		FENBefore: rec.FENBefore,
		FENAfter:  rec.FENAfter,
		CreatedAt: rec.CreatedAt,
	}
	s.history[gameID] = append(s.history[gameID], item)

	return s.history[gameID], nil
}

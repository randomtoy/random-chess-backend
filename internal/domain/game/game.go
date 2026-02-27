package game

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/notnil/chess"
)

// Status values match the contract enum.
type Status string

const (
	StatusWaiting   Status = "waiting"
	StatusOngoing   Status = "ongoing"
	StatusCheckmate Status = "checkmate"
	StatusStalemate Status = "stalemate"
	StatusDraw      Status = "draw"
	StatusResigned  Status = "resigned"
)

// Result values match the contract enum.
type Result string

const (
	ResultWhite Result = "1-0"
	ResultBlack Result = "0-1"
	ResultDraw  Result = "1/2-1/2"
)

// Sentinel errors returned by ApplyMove; transport layer maps these to HTTP codes.
var (
	ErrInvalidUCI     = errors.New("invalid_uci")
	ErrIllegalMove    = errors.New("illegal_move")
	ErrGameNotOngoing = errors.New("game_not_ongoing")
)

// Game is the domain entity. All pointer fields are nullable in the contract.
type Game struct {
	ID           uuid.UUID
	Status       Status
	Result       *Result
	FEN          string
	SideToMove   string
	PlyCount     int
	LastMoveUCI  *string
	LastMoveAt   *time.Time
	StateVersion int
	CreatedAt    time.Time
	UpdatedAt    time.Time

	// chessGame holds live chess state and is never serialized directly.
	chessGame *chess.Game
}

// MoveRecord is the accepted-move detail returned inside SubmitMoveAccepted.
type MoveRecord struct {
	ID        uuid.UUID
	UCI       string
	FENBefore string
	FENAfter  string
	CreatedAt time.Time
}

// MoveHistoryItem is one entry in a game's persisted move history.
type MoveHistoryItem struct {
	Ply       int
	UCI       string
	FromSq    string
	ToSq      string
	Promotion *string
	ClientID  uuid.UUID
	FENBefore string
	FENAfter  string
	CreatedAt time.Time
}

// NewGame creates a Game seeded from the standard starting position.
func NewGame(id uuid.UUID, now time.Time) *Game {
	cg := chess.NewGame(chess.UseNotation(chess.UCINotation{}))
	return fromChessGame(id, cg, now)
}

func fromChessGame(id uuid.UUID, cg *chess.Game, now time.Time) *Game {
	pos := cg.Position()
	g := &Game{
		ID:           id,
		Status:       StatusOngoing,
		Result:       nil,
		FEN:          pos.String(),
		SideToMove:   colorName(pos.Turn()),
		PlyCount:     len(cg.Moves()),
		LastMoveUCI:  nil,
		LastMoveAt:   nil,
		StateVersion: 0,
		CreatedAt:    now,
		UpdatedAt:    now,
		chessGame:    cg,
	}
	return g
}

// ApplyMove validates the UCI move against the current position and returns a
// new *Game with all fields updated. The receiver is never mutated, so the
// caller can safely pass the new game to SaveIfVersion while the store still
// holds the original pointer for CAS comparison.
//
// Returns:
//   - ErrGameNotOngoing — game has already ended
//   - ErrInvalidUCI     — string is not valid UCI syntax
//   - ErrIllegalMove    — syntactically valid but not legal in this position
func (g *Game) ApplyMove(uci string, now time.Time) (*Game, MoveRecord, error) {
	if g.Status != StatusOngoing && g.Status != StatusWaiting {
		return nil, MoveRecord{}, ErrGameNotOngoing
	}

	if !isValidUCISyntax(uci) {
		return nil, MoveRecord{}, ErrInvalidUCI
	}

	// Build a fresh chess.Game from the stored FEN so the receiver is untouched.
	fenOpt, err := chess.FEN(g.FEN)
	if err != nil {
		return nil, MoveRecord{}, ErrIllegalMove
	}
	newCG := chess.NewGame(fenOpt, chess.UseNotation(chess.UCINotation{}))

	fenBefore := g.FEN

	if err := newCG.MoveStr(uci); err != nil {
		return nil, MoveRecord{}, ErrIllegalMove
	}

	pos := newCG.Position()
	fenAfter := pos.String()
	uciCopy := uci
	nowCopy := now

	newG := &Game{
		ID:           g.ID,
		FEN:          fenAfter,
		SideToMove:   colorName(pos.Turn()),
		PlyCount:     g.PlyCount + 1,
		LastMoveUCI:  &uciCopy,
		LastMoveAt:   &nowCopy,
		StateVersion: g.StateVersion + 1,
		CreatedAt:    g.CreatedAt,
		UpdatedAt:    now,
		chessGame:    newCG,
	}
	newG.Status, newG.Result = outcomeToStatus(newCG.Outcome(), newCG.Method())

	rec := MoveRecord{
		ID:        uuid.New(),
		UCI:       uci,
		FENBefore: fenBefore,
		FENAfter:  fenAfter,
		CreatedAt: now,
	}
	return newG, rec, nil
}

// isValidUCISyntax returns true iff s is valid UCI move notation:
// [a-h][1-8][a-h][1-8] with an optional promotion piece [qrbn].
func isValidUCISyntax(s string) bool {
	if len(s) < 4 || len(s) > 5 {
		return false
	}
	if s[0] < 'a' || s[0] > 'h' {
		return false
	}
	if s[1] < '1' || s[1] > '8' {
		return false
	}
	if s[2] < 'a' || s[2] > 'h' {
		return false
	}
	if s[3] < '1' || s[3] > '8' {
		return false
	}
	if len(s) == 5 {
		switch s[4] {
		case 'q', 'r', 'b', 'n':
		default:
			return false
		}
	}
	return true
}

func colorName(c chess.Color) string {
	if c == chess.White {
		return "white"
	}
	return "black"
}

func outcomeToStatus(outcome chess.Outcome, method chess.Method) (Status, *Result) {
	switch outcome {
	case chess.WhiteWon:
		r := ResultWhite
		if method == chess.Checkmate {
			return StatusCheckmate, &r
		}
		return StatusResigned, &r
	case chess.BlackWon:
		r := ResultBlack
		if method == chess.Checkmate {
			return StatusCheckmate, &r
		}
		return StatusResigned, &r
	case chess.Draw:
		r := ResultDraw
		if method == chess.Stalemate {
			return StatusStalemate, &r
		}
		return StatusDraw, &r
	default:
		return StatusOngoing, nil
	}
}

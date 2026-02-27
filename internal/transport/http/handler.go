package http

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/randomtoy/random-chess-backend/internal/domain/game"
	"github.com/randomtoy/random-chess-backend/internal/ports"
	"github.com/randomtoy/random-chess-backend/internal/usecase"
)

// moveHistoryJSON is the wire representation of a single move in history.
type moveHistoryJSON struct {
	Ply       int        `json:"ply"`
	UCI       string     `json:"uci"`
	From      string     `json:"from"`
	To        string     `json:"to"`
	Promotion *string    `json:"promotion,omitempty"`
	ClientID  string     `json:"client_id"`
	FENBefore string     `json:"fen_before"`
	FENAfter  string     `json:"fen_after"`
	CreatedAt time.Time  `json:"created_at"`
}

// gameJSON is the wire representation of domain/game.Game (matches contract,
// extended with move_history).
type gameJSON struct {
	GameID       string            `json:"game_id"`
	Status       string            `json:"status"`
	Result       *string           `json:"result"`
	FEN          string            `json:"fen"`
	SideToMove   string            `json:"side_to_move"`
	PlyCount     int               `json:"ply_count"`
	LastMoveUCI  *string           `json:"last_move_uci"`
	LastMoveAt   *time.Time        `json:"last_move_at"`
	StateVersion int               `json:"state_version"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
	MoveHistory  []moveHistoryJSON `json:"move_history"`
}

func toMoveHistoryJSON(items []game.MoveHistoryItem) []moveHistoryJSON {
	out := make([]moveHistoryJSON, len(items))
	for i, item := range items {
		out[i] = moveHistoryJSON{
			Ply:       item.Ply,
			UCI:       item.UCI,
			From:      item.FromSq,
			To:        item.ToSq,
			Promotion: item.Promotion,
			ClientID:  item.ClientID.String(),
			FENBefore: item.FENBefore,
			FENAfter:  item.FENAfter,
			CreatedAt: item.CreatedAt,
		}
	}
	return out
}

func toGameJSON(g *game.Game, history []game.MoveHistoryItem) *gameJSON {
	var result *string
	if g.Result != nil {
		s := string(*g.Result)
		result = &s
	}
	return &gameJSON{
		GameID:       g.ID.String(),
		Status:       string(g.Status),
		Result:       result,
		FEN:          g.FEN,
		SideToMove:   g.SideToMove,
		PlyCount:     g.PlyCount,
		LastMoveUCI:  g.LastMoveUCI,
		LastMoveAt:   g.LastMoveAt,
		StateVersion: g.StateVersion,
		CreatedAt:    g.CreatedAt,
		UpdatedAt:    g.UpdatedAt,
		MoveHistory:  toMoveHistoryJSON(history),
	}
}

// parseClientID reads and validates the X-Client-Id header.
// Returns uuid.Nil and writes a 400 response if the header is missing/invalid.
func parseClientID(c echo.Context) (uuid.UUID, error) {
	raw := c.Request().Header.Get("X-Client-Id")
	if raw == "" {
		return uuid.Nil, c.JSON(http.StatusBadRequest, Problem{
			Type:   errBase + "/missing-client-id",
			Title:  "Bad Request",
			Status: http.StatusBadRequest,
			Detail: "X-Client-Id header is required (UUID).",
		})
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, c.JSON(http.StatusBadRequest, Problem{
			Type:   errBase + "/invalid-client-id",
			Title:  "Bad Request",
			Status: http.StatusBadRequest,
			Detail: "X-Client-Id must be a valid UUID.",
		})
	}
	return id, nil
}

// Handlers holds all usecase dependencies.
type Handlers struct {
	assigner  *usecase.Assigner
	nextGame  *usecase.NextGame
	getter    *usecase.GameGetter
	submitter *usecase.MoveSubmitter
}

func NewHandlers(
	assigner *usecase.Assigner,
	nextGame *usecase.NextGame,
	getter *usecase.GameGetter,
	submitter *usecase.MoveSubmitter,
) *Handlers {
	return &Handlers{assigner: assigner, nextGame: nextGame, getter: getter, submitter: submitter}
}

func (h *Handlers) handleHealthz(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]bool{"ok": true})
}

// handleGetAssigned is the legacy endpoint — backward compatible, no client tracking.
func (h *Handlers) handleGetAssigned(c echo.Context) error {
	ip := c.RealIP()
	token := c.Request().Header.Get("X-Client-Token")

	res, err := h.assigner.Assign(c.Request().Context(), ip, token)
	if err != nil {
		return writeErr(c, err)
	}

	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(http.StatusOK, map[string]any{
		"game": toGameJSON(res.Game, []game.MoveHistoryItem{}),
		"assignment": map[string]any{
			"assignment_id": res.AssignmentID.String(),
			"assigned_at":   res.AssignedAt,
		},
	})
}

// handleGetNext returns a game that this client has not yet played, claiming it
// for the client. Requires X-Client-Id header.
func (h *Handlers) handleGetNext(c echo.Context) error {
	clientID, err := parseClientID(c)
	if err != nil {
		return err // response already written
	}

	ip := c.RealIP()
	token := c.Request().Header.Get("X-Client-Token")

	res, err := h.nextGame.GetNext(c.Request().Context(), ip, token, clientID)
	if err != nil {
		return writeErr(c, err)
	}

	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(http.StatusOK, map[string]any{
		"game": toGameJSON(res.Game, res.History),
	})
}

func (h *Handlers) handleGetGame(c echo.Context) error {
	ip := c.RealIP()
	token := c.Request().Header.Get("X-Client-Token")

	id, err := uuid.Parse(c.Param("game_id"))
	if err != nil {
		return writeErr(c, ports.ErrNotFound)
	}

	g, hist, err := h.getter.GetGame(c.Request().Context(), ip, token, id)
	if err != nil {
		return writeErr(c, err)
	}

	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(http.StatusOK, toGameJSON(g, hist))
}

func (h *Handlers) handleSubmitMove(c echo.Context) error {
	ip := c.RealIP()
	token := c.Request().Header.Get("X-Client-Token")

	clientID, err := parseClientID(c)
	if err != nil {
		return err // response already written
	}

	id, err := uuid.Parse(c.Param("game_id"))
	if err != nil {
		return writeErr(c, ports.ErrNotFound)
	}

	var body struct {
		// Legacy form: single UCI string.
		UCI string `json:"uci"`
		// New form: from/to/promotion.
		From      string  `json:"from"`
		To        string  `json:"to"`
		Promotion *string `json:"promotion"`
		// Optimistic concurrency.
		ExpectedVersion int     `json:"expected_version"`
		ClientNonce     *string `json:"client_nonce"`
	}
	if bindErr := c.Bind(&body); bindErr != nil {
		return writeErr(c, bindErr)
	}

	// Resolve UCI: prefer from/to over the uci field.
	uci := body.UCI
	if body.From != "" && body.To != "" {
		uci = body.From + body.To
		if body.Promotion != nil {
			uci += *body.Promotion
		}
	}
	if uci == "" {
		return writeErr(c, game.ErrInvalidUCI)
	}

	req := usecase.SubmitMoveRequest{
		UCI:             uci,
		ExpectedVersion: body.ExpectedVersion,
		ClientNonce:     body.ClientNonce,
	}

	res, err := h.submitter.SubmitMove(c.Request().Context(), ip, token, id, clientID, req)
	if err != nil {
		return writeErr(c, err)
	}

	var nextHint any
	if res.ShouldFetchNext {
		nextHint = map[string]any{"should_fetch_next": true}
	}

	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(http.StatusOK, map[string]any{
		"accepted": true,
		"move": map[string]any{
			"move_id":    res.Move.ID.String(),
			"uci":        res.Move.UCI,
			"fen_before": res.Move.FENBefore,
			"fen_after":  res.Move.FENAfter,
			"created_at": res.Move.CreatedAt,
		},
		"game":                 toGameJSON(res.Game, res.History),
		"next_assignment_hint": nextHint,
	})
}

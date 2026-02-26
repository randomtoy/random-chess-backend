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

// gameJSON is the wire representation of domain/game.Game (matches contract).
type gameJSON struct {
	GameID       string     `json:"game_id"`
	Status       string     `json:"status"`
	Result       *string    `json:"result"`
	FEN          string     `json:"fen"`
	SideToMove   string     `json:"side_to_move"`
	PlyCount     int        `json:"ply_count"`
	LastMoveUCI  *string    `json:"last_move_uci"`
	LastMoveAt   *time.Time `json:"last_move_at"`
	StateVersion int        `json:"state_version"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

func toGameJSON(g *game.Game) *gameJSON {
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
	}
}

// Handlers holds all usecase dependencies.
type Handlers struct {
	assigner  *usecase.Assigner
	getter    *usecase.GameGetter
	submitter *usecase.MoveSubmitter
}

func NewHandlers(
	assigner *usecase.Assigner,
	getter *usecase.GameGetter,
	submitter *usecase.MoveSubmitter,
) *Handlers {
	return &Handlers{assigner: assigner, getter: getter, submitter: submitter}
}

func (h *Handlers) handleHealthz(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handlers) handleGetAssigned(c echo.Context) error {
	ip := c.RealIP()
	token := c.Request().Header.Get("X-Client-Token")

	res, err := h.assigner.Assign(c.Request().Context(), ip, token)
	if err != nil {
		return writeErr(c, err)
	}

	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(http.StatusOK, map[string]any{
		"game": toGameJSON(res.Game),
		"assignment": map[string]any{
			"assignment_id": res.AssignmentID.String(),
			"assigned_at":   res.AssignedAt,
		},
	})
}

func (h *Handlers) handleGetGame(c echo.Context) error {
	ip := c.RealIP()
	token := c.Request().Header.Get("X-Client-Token")

	id, err := uuid.Parse(c.Param("game_id"))
	if err != nil {
		return writeErr(c, ports.ErrNotFound)
	}

	g, err := h.getter.GetGame(c.Request().Context(), ip, token, id)
	if err != nil {
		return writeErr(c, err)
	}

	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(http.StatusOK, toGameJSON(g))
}

func (h *Handlers) handleSubmitMove(c echo.Context) error {
	ip := c.RealIP()
	token := c.Request().Header.Get("X-Client-Token")

	id, err := uuid.Parse(c.Param("game_id"))
	if err != nil {
		return writeErr(c, ports.ErrNotFound)
	}

	var body struct {
		UCI             string  `json:"uci"`
		ExpectedVersion int     `json:"expected_version"`
		ClientNonce     *string `json:"client_nonce"`
	}
	if bindErr := c.Bind(&body); bindErr != nil {
		return writeErr(c, bindErr)
	}
	if body.UCI == "" {
		return writeErr(c, game.ErrInvalidUCI)
	}

	req := usecase.SubmitMoveRequest{
		UCI:             body.UCI,
		ExpectedVersion: body.ExpectedVersion,
		ClientNonce:     body.ClientNonce,
	}

	res, err := h.submitter.SubmitMove(c.Request().Context(), ip, token, id, req)
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
		"game":                 toGameJSON(res.Game),
		"next_assignment_hint": nextHint,
	})
}

package http

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/randomtoy/random-chess-backend/internal/domain/game"
	"github.com/randomtoy/random-chess-backend/internal/ports"
	"github.com/randomtoy/random-chess-backend/internal/usecase"
)

const errBase = "https://errors.random-chess.local"

// Problem matches the contract Problem schema.
type Problem struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
}

// IllegalMoveProblem matches the contract IllegalMoveProblem schema.
type IllegalMoveProblem struct {
	Problem
	Code string    `json:"code"`
	Game *gameJSON `json:"game,omitempty"`
}

// writeErr maps a domain/usecase error to the correct HTTP response.
func writeErr(c echo.Context, err error) error {
	switch {
	case errors.Is(err, ports.ErrNotFound):
		return c.JSON(http.StatusNotFound, Problem{
			Type:   errBase + "/not-found",
			Title:  "Not Found",
			Status: http.StatusNotFound,
			Detail: "Resource not found.",
		})
	case errors.Is(err, ports.ErrVersionConflict):
		return c.JSON(http.StatusConflict, Problem{
			Type:   errBase + "/conflict",
			Title:  "Conflict",
			Status: http.StatusConflict,
			Detail: "Game state changed; refresh and retry with new expected_version.",
		})
	case errors.Is(err, usecase.ErrRateLimited):
		c.Response().Header().Set("Retry-After", "2")
		return c.JSON(http.StatusTooManyRequests, Problem{
			Type:   errBase + "/rate-limited",
			Title:  "Too Many Requests",
			Status: http.StatusTooManyRequests,
			Detail: "Rate limit exceeded. Try again later.",
		})
	case errors.Is(err, game.ErrGameNotOngoing):
		return c.JSON(http.StatusUnprocessableEntity, IllegalMoveProblem{
			Problem: Problem{
				Type:   errBase + "/illegal-move",
				Title:  "Unprocessable Entity",
				Status: http.StatusUnprocessableEntity,
				Detail: "Game is not ongoing.",
			},
			Code: "game_not_ongoing",
		})
	case errors.Is(err, game.ErrInvalidUCI):
		return c.JSON(http.StatusUnprocessableEntity, IllegalMoveProblem{
			Problem: Problem{
				Type:   errBase + "/illegal-move",
				Title:  "Unprocessable Entity",
				Status: http.StatusUnprocessableEntity,
				Detail: "Move string is not valid UCI notation.",
			},
			Code: "invalid_uci",
		})
	case errors.Is(err, game.ErrIllegalMove):
		return c.JSON(http.StatusUnprocessableEntity, IllegalMoveProblem{
			Problem: Problem{
				Type:   errBase + "/illegal-move",
				Title:  "Unprocessable Entity",
				Status: http.StatusUnprocessableEntity,
				Detail: "Move is not legal in the current position.",
			},
			Code: "illegal_move",
		})
	default:
		return c.JSON(http.StatusInternalServerError, Problem{
			Type:   errBase + "/internal",
			Title:  "Internal Server Error",
			Status: http.StatusInternalServerError,
			Detail: "Unexpected error.",
		})
	}
}

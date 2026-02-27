package http

import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// New constructs and returns a configured Echo instance.
func New(h *Handlers) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"https://chess.randomtoy.dev"},
		AllowMethods: []string{"GET", "POST", "OPTIONS"},
		AllowHeaders: []string{"Content-Type", "X-Client-Token", "X-Client-Id"},
	}))
	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())

	e.GET("/api/v1/healthz", h.handleHealthz)
	e.GET("/api/v1/games/assigned", h.handleGetAssigned)
	e.GET("/api/v1/games/next", h.handleGetNext)
	e.GET("/api/v1/games/:game_id", h.handleGetGame)
	e.POST("/api/v1/games/:game_id/moves", h.handleSubmitMove)

	return e
}

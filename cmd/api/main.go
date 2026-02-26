package main

import (
	"context"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/randomtoy/random-chess-backend/internal/adapters/memory"
	pgstore "github.com/randomtoy/random-chess-backend/internal/adapters/postgres"
	"github.com/randomtoy/random-chess-backend/internal/config"
	"github.com/randomtoy/random-chess-backend/internal/domain/game"
	"github.com/randomtoy/random-chess-backend/internal/ports"
	transporthttp "github.com/randomtoy/random-chess-backend/internal/transport/http"
	"github.com/randomtoy/random-chess-backend/internal/usecase"
)

func main() {
	cfg := config.Load()

	var store ports.GameStore
	rl := memory.AlwaysAllow{}

	if cfg.DatabaseURL != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
		cancel()
		if err != nil {
			log.Fatalf("pgxpool.New: %v", err)
		}
		defer pool.Close()

		pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := pool.Ping(pingCtx); err != nil {
			log.Fatalf("db ping: %v", err)
		}
		pingCancel()
		log.Println("connected to database")

		pg := pgstore.New(pool)
		seedIfEmpty(pg, 5)
		store = pg
	} else {
		store = memory.New(5)
	}

	h := transporthttp.NewHandlers(
		usecase.NewAssigner(store, rl),
		usecase.NewGameGetter(store, rl),
		usecase.NewMoveSubmitter(store, rl),
	)

	e := transporthttp.New(h)
	log.Printf("starting on :%s", cfg.Port)
	log.Fatal(e.Start(":" + cfg.Port))
}

func seedIfEmpty(pg *pgstore.Store, count int) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	games, err := pg.ListOngoing(ctx)
	if err != nil {
		log.Printf("seed check failed: %v", err)
		return
	}
	if len(games) > 0 {
		return
	}

	now := time.Now()
	for i := 0; i < count; i++ {
		g := game.NewGame(uuid.New(), now)
		if err := pg.Insert(ctx, g); err != nil {
			log.Printf("seed insert %d: %v", i, err)
		}
	}
	log.Printf("seeded %d games", count)
}

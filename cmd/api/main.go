package main

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/randomtoy/random-chess-backend/internal/adapters/memory"
	pgstore "github.com/randomtoy/random-chess-backend/internal/adapters/postgres"
	"github.com/randomtoy/random-chess-backend/internal/config"
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
		seedIfEmpty(pg, cfg.GameCreateBatchSize)
		store = pg
	} else {
		store = memory.New(cfg.GameCreateBatchSize)
	}

	h := transporthttp.NewHandlers(
		usecase.NewAssigner(store, rl),
		usecase.NewNextGame(store, rl, cfg.GameCreateBatchSize),
		usecase.NewGameGetter(store, rl),
		usecase.NewMoveSubmitter(store, rl),
	)

	e := transporthttp.New(h)
	log.Printf("starting on :%s", cfg.Port)
	log.Fatal(e.Start(":" + cfg.Port))
}

// seedIfEmpty creates a batch of waiting games if the DB has no active games.
func seedIfEmpty(store ports.GameStore, batchSize int) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	has, err := store.HasActiveGames(ctx)
	if err != nil {
		log.Printf("seed check failed: %v", err)
		return
	}
	if has {
		return
	}

	if err := store.CreateWaitingBatch(ctx, batchSize); err != nil {
		log.Printf("seed batch failed: %v", err)
		return
	}
	log.Printf("seeded %d waiting games", batchSize)
}

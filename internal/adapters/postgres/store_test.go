//go:build integration

package postgres_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	pgstore "github.com/randomtoy/random-chess-backend/internal/adapters/postgres"
	"github.com/randomtoy/random-chess-backend/internal/db"
	"github.com/randomtoy/random-chess-backend/internal/domain/game"
	"github.com/randomtoy/random-chess-backend/internal/ports"
)

func setupStore(t *testing.T) *pgstore.Store {
	t.Helper()
	ctx := context.Background()

	ctr, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("testdb"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = ctr.Terminate(ctx) })

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	// Run migrations via goose.
	sqlDB, err := sql.Open("pgx", connStr)
	if err != nil {
		t.Fatalf("open sql.DB: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	goose.SetBaseFS(db.Migrations)
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatalf("goose set dialect: %v", err)
	}
	if err := goose.Up(sqlDB, "migrations"); err != nil {
		t.Fatalf("goose up: %v", err)
	}

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)

	return pgstore.New(pool)
}

func newTestGame(t *testing.T) *game.Game {
	t.Helper()
	return game.NewGame(uuid.New(), time.Now().UTC().Truncate(time.Millisecond))
}

func TestGetByID_NotFound(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	_, err := s.GetByID(ctx, uuid.New())
	if err != ports.ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestInsertAndGetByID(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	g := newTestGame(t)
	if err := s.Insert(ctx, g); err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err := s.GetByID(ctx, g.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != g.ID {
		t.Errorf("id: want %v, got %v", g.ID, got.ID)
	}
	if got.FEN != g.FEN {
		t.Errorf("fen: want %q, got %q", g.FEN, got.FEN)
	}
	if got.StateVersion != g.StateVersion {
		t.Errorf("state_version: want %d, got %d", g.StateVersion, got.StateVersion)
	}
	if got.Status != game.StatusOngoing {
		t.Errorf("status: want ongoing, got %q", got.Status)
	}
}

func TestListOngoing(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if err := s.Insert(ctx, newTestGame(t)); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}

	games, err := s.ListOngoing(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(games) != 3 {
		t.Fatalf("want 3 ongoing games, got %d", len(games))
	}
}

func TestSaveIfVersion_OK(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	g := newTestGame(t)
	if err := s.Insert(ctx, g); err != nil {
		t.Fatalf("insert: %v", err)
	}

	newG, _, err := g.ApplyMove("e2e4", time.Now().UTC())
	if err != nil {
		t.Fatalf("apply move: %v", err)
	}

	if err := s.SaveIfVersion(ctx, newG, 0); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := s.GetByID(ctx, g.ID)
	if err != nil {
		t.Fatalf("get after save: %v", err)
	}
	if got.StateVersion != 1 {
		t.Errorf("state_version: want 1, got %d", got.StateVersion)
	}
	if got.FEN == g.FEN {
		t.Error("FEN should change after a move")
	}
}

func TestSaveIfVersion_Conflict(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	g := newTestGame(t)
	if err := s.Insert(ctx, g); err != nil {
		t.Fatalf("insert: %v", err)
	}

	newG, _, err := g.ApplyMove("e2e4", time.Now().UTC())
	if err != nil {
		t.Fatalf("apply move: %v", err)
	}

	// Wrong expected version → conflict.
	if err := s.SaveIfVersion(ctx, newG, 99); err != ports.ErrVersionConflict {
		t.Fatalf("want ErrVersionConflict, got %v", err)
	}
}

// ── New integration tests ──────────────────────────────────────────────────────

func TestHasActiveGames(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	has, err := s.HasActiveGames(ctx)
	if err != nil {
		t.Fatalf("HasActiveGames: %v", err)
	}
	if has {
		t.Fatal("expected no active games on empty DB")
	}

	if err := s.Insert(ctx, newTestGame(t)); err != nil {
		t.Fatalf("insert: %v", err)
	}

	has, err = s.HasActiveGames(ctx)
	if err != nil {
		t.Fatalf("HasActiveGames after insert: %v", err)
	}
	if !has {
		t.Fatal("expected active games after insert")
	}
}

func TestCreateWaitingBatch(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	if err := s.CreateWaitingBatch(ctx, 5); err != nil {
		t.Fatalf("CreateWaitingBatch: %v", err)
	}

	has, err := s.HasActiveGames(ctx)
	if err != nil {
		t.Fatalf("HasActiveGames: %v", err)
	}
	if !has {
		t.Fatal("expected active games after batch creation")
	}
}

func TestClaimNextGame_NeverRepeatsSameClient(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	// Create 2 waiting games.
	if err := s.CreateWaitingBatch(ctx, 2); err != nil {
		t.Fatalf("batch: %v", err)
	}

	clientID := uuid.New()

	g1, hist1, err := s.ClaimNextGame(ctx, clientID)
	if err != nil {
		t.Fatalf("claim1: %v", err)
	}
	if hist1 == nil {
		t.Fatal("history must not be nil")
	}

	g2, _, err := s.ClaimNextGame(ctx, clientID)
	if err != nil {
		t.Fatalf("claim2: %v", err)
	}

	if g1.ID == g2.ID {
		t.Fatalf("same client received the same game twice: %s", g1.ID)
	}

	// Third claim should fail — only 2 games exist.
	_, _, err = s.ClaimNextGame(ctx, clientID)
	if err != ports.ErrNoGamesAvailable {
		t.Fatalf("want ErrNoGamesAvailable, got %v", err)
	}
}

func TestPersistMove_Full(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	if err := s.CreateWaitingBatch(ctx, 1); err != nil {
		t.Fatalf("batch: %v", err)
	}
	clientID := uuid.New()
	g, _, err := s.ClaimNextGame(ctx, clientID)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}

	newGame, rec, err := g.ApplyMove("e2e4", time.Now().UTC())
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	ply := newGame.PlyCount - 1

	hist, err := s.PersistMove(ctx, g.ID, clientID, newGame, rec, ply)
	if err != nil {
		t.Fatalf("persist: %v", err)
	}
	if len(hist) != 1 {
		t.Fatalf("want 1 history item, got %d", len(hist))
	}
	if hist[0].UCI != "e2e4" {
		t.Errorf("history uci: want e2e4, got %q", hist[0].UCI)
	}

	// Second move attempt by same client → ErrAlreadyMoved.
	newGame2, rec2, err := newGame.ApplyMove("e7e5", time.Now().UTC())
	if err != nil {
		t.Fatalf("apply2: %v", err)
	}
	_, err = s.PersistMove(ctx, g.ID, clientID, newGame2, rec2, newGame2.PlyCount-1)
	if err != ports.ErrAlreadyMoved {
		t.Fatalf("want ErrAlreadyMoved, got %v", err)
	}
}

func TestPersistMove_NotAssigned(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	g := newTestGame(t)
	if err := s.Insert(ctx, g); err != nil {
		t.Fatalf("insert: %v", err)
	}

	unassigned := uuid.New()
	newGame, rec, err := g.ApplyMove("e2e4", time.Now().UTC())
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	_, err = s.PersistMove(ctx, g.ID, unassigned, newGame, rec, 0)
	if err != ports.ErrNotAssigned {
		t.Fatalf("want ErrNotAssigned, got %v", err)
	}
}

func TestGetGameWithHistory(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	if err := s.CreateWaitingBatch(ctx, 1); err != nil {
		t.Fatalf("batch: %v", err)
	}
	clientID := uuid.New()
	g, _, err := s.ClaimNextGame(ctx, clientID)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}

	newGame, rec, err := g.ApplyMove("e2e4", time.Now().UTC())
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if _, err := s.PersistMove(ctx, g.ID, clientID, newGame, rec, 0); err != nil {
		t.Fatalf("persist: %v", err)
	}

	got, hist, err := s.GetGameWithHistory(ctx, g.ID)
	if err != nil {
		t.Fatalf("getWithHistory: %v", err)
	}
	if got.StateVersion != 1 {
		t.Errorf("state_version: want 1, got %d", got.StateVersion)
	}
	if len(hist) != 1 {
		t.Fatalf("want 1 history item, got %d", len(hist))
	}
	if hist[0].Ply != 0 {
		t.Errorf("ply: want 0, got %d", hist[0].Ply)
	}
}

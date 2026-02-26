package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/randomtoy/random-chess-backend/internal/adapters/memory"
	"github.com/randomtoy/random-chess-backend/internal/usecase"
	transporthttp "github.com/randomtoy/random-chess-backend/internal/transport/http"
)

func newTestServer(t *testing.T) *transporthttp.Handlers {
	t.Helper()
	store := memory.New(3)
	rl := memory.AlwaysAllow{}
	return transporthttp.NewHandlers(
		usecase.NewAssigner(store, rl),
		usecase.NewGameGetter(store, rl),
		usecase.NewMoveSubmitter(store, rl),
	)
}

func doRequest(t *testing.T, h *transporthttp.Handlers, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	transporthttp.New(h).ServeHTTP(rec, req)
	return rec
}

func TestHealthz(t *testing.T) {
	h := newTestServer(t)
	rec := doRequest(t, h, http.MethodGet, "/api/v1/healthz", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]bool
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp["ok"] {
		t.Fatalf("expected ok:true, got %v", resp)
	}
}

func TestGetAssigned_OngoingGame(t *testing.T) {
	h := newTestServer(t)
	rec := doRequest(t, h, http.MethodGet, "/api/v1/games/assigned", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Game struct {
			Status string `json:"status"`
		} `json:"game"`
		Assignment struct {
			AssignmentID string `json:"assignment_id"`
		} `json:"assignment"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Game.Status != "ongoing" {
		t.Fatalf("expected status ongoing, got %q", resp.Game.Status)
	}
	if resp.Assignment.AssignmentID == "" {
		t.Fatal("expected non-empty assignment_id")
	}
}

// getAssignedGameID fetches /games/assigned and returns the game_id.
func getAssignedGameID(t *testing.T, h *transporthttp.Handlers) string {
	t.Helper()
	rec := doRequest(t, h, http.MethodGet, "/api/v1/games/assigned", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("assigned: expected 200, got %d", rec.Code)
	}
	var resp struct {
		Game struct {
			GameID string `json:"game_id"`
		} `json:"game"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp.Game.GameID
}

func TestSubmitMove_Legal(t *testing.T) {
	h := newTestServer(t)
	gameID := getAssignedGameID(t, h)

	// Fetch game to get starting FEN and state_version.
	rec := doRequest(t, h, http.MethodGet, "/api/v1/games/"+gameID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get game: expected 200, got %d", rec.Code)
	}
	var initial struct {
		FEN          string `json:"fen"`
		StateVersion int    `json:"state_version"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&initial); err != nil {
		t.Fatalf("decode: %v", err)
	}

	rec = doRequest(t, h, http.MethodPost, "/api/v1/games/"+gameID+"/moves", map[string]any{
		"uci":              "e2e4",
		"expected_version": initial.StateVersion,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("submit: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Accepted bool `json:"accepted"`
		Move     struct {
			FENBefore string `json:"fen_before"`
			FENAfter  string `json:"fen_after"`
		} `json:"move"`
		Game struct {
			StateVersion int    `json:"state_version"`
			FEN          string `json:"fen"`
		} `json:"game"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Accepted {
		t.Fatal("expected accepted:true")
	}
	if resp.Game.StateVersion != initial.StateVersion+1 {
		t.Fatalf("state_version: want %d, got %d", initial.StateVersion+1, resp.Game.StateVersion)
	}
	if resp.Game.FEN == initial.FEN {
		t.Fatal("FEN should change after a legal move")
	}
	if resp.Move.FENBefore != initial.FEN {
		t.Fatalf("fen_before mismatch: want %q, got %q", initial.FEN, resp.Move.FENBefore)
	}
}

func TestSubmitMove_IllegalMove(t *testing.T) {
	h := newTestServer(t)
	gameID := getAssignedGameID(t, h)

	// e2e5: valid UCI syntax but a pawn cannot jump 3 squares.
	rec := doRequest(t, h, http.MethodPost, "/api/v1/games/"+gameID+"/moves", map[string]any{
		"uci":              "e2e5",
		"expected_version": 0,
	})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Code != "illegal_move" {
		t.Fatalf("expected code illegal_move, got %q", resp.Code)
	}
}

func TestSubmitMove_InvalidUCI(t *testing.T) {
	h := newTestServer(t)
	gameID := getAssignedGameID(t, h)

	// "zzzz" is not valid UCI notation.
	rec := doRequest(t, h, http.MethodPost, "/api/v1/games/"+gameID+"/moves", map[string]any{
		"uci":              "zzzz",
		"expected_version": 0,
	})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Code != "invalid_uci" {
		t.Fatalf("expected code invalid_uci, got %q", resp.Code)
	}
}

func TestSubmitMove_VersionMismatch(t *testing.T) {
	h := newTestServer(t)
	gameID := getAssignedGameID(t, h)

	rec := doRequest(t, h, http.MethodPost, "/api/v1/games/"+gameID+"/moves", map[string]any{
		"uci":              "e2e4",
		"expected_version": 99, // stale
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

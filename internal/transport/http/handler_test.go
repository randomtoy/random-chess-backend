package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/randomtoy/random-chess-backend/internal/adapters/memory"
	transporthttp "github.com/randomtoy/random-chess-backend/internal/transport/http"
	"github.com/randomtoy/random-chess-backend/internal/usecase"
)

const testBatchSize = 3

func newTestServer(t *testing.T) *transporthttp.Handlers {
	t.Helper()
	return newTestServerWithStore(t, memory.New(testBatchSize))
}

func newTestServerWithStore(t *testing.T, store *memory.Store) *transporthttp.Handlers {
	t.Helper()
	rl := memory.AlwaysAllow{}
	return transporthttp.NewHandlers(
		usecase.NewAssigner(store, rl),
		usecase.NewNextGame(store, rl, testBatchSize),
		usecase.NewGameGetter(store, rl),
		usecase.NewMoveSubmitter(store, rl),
	)
}

func doRequest(t *testing.T, h *transporthttp.Handlers, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
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
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	transporthttp.New(h).ServeHTTP(rec, req)
	return rec
}

// getNextGame calls GET /api/v1/games/next and returns gameID + stateVersion.
func getNextGame(t *testing.T, h *transporthttp.Handlers, clientID string) (gameID string, stateVersion int) {
	t.Helper()
	rec := doRequest(t, h, http.MethodGet, "/api/v1/games/next", nil, map[string]string{
		"X-Client-Id": clientID,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /games/next: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Game struct {
			GameID       string `json:"game_id"`
			StateVersion int    `json:"state_version"`
		} `json:"game"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp.Game.GameID, resp.Game.StateVersion
}

// ── Existing tests (updated) ──────────────────────────────────────────────────

func TestHealthz(t *testing.T) {
	h := newTestServer(t)
	rec := doRequest(t, h, http.MethodGet, "/api/v1/healthz", nil, nil)
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
	rec := doRequest(t, h, http.MethodGet, "/api/v1/games/assigned", nil, nil)
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

func TestSubmitMove_Legal(t *testing.T) {
	h := newTestServer(t)
	clientID := uuid.New().String()
	gameID, ver := getNextGame(t, h, clientID)

	rec := doRequest(t, h, http.MethodPost, "/api/v1/games/"+gameID+"/moves",
		map[string]any{"uci": "e2e4", "expected_version": ver},
		map[string]string{"X-Client-Id": clientID},
	)
	if rec.Code != http.StatusOK {
		t.Fatalf("submit: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Accepted bool `json:"accepted"`
		Game     struct {
			StateVersion int    `json:"state_version"`
			FEN          string `json:"fen"`
			MoveHistory  []struct {
				Ply int    `json:"ply"`
				UCI string `json:"uci"`
			} `json:"move_history"`
		} `json:"game"`
		Move struct {
			FENBefore string `json:"fen_before"`
			FENAfter  string `json:"fen_after"`
		} `json:"move"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Accepted {
		t.Fatal("expected accepted:true")
	}
	if resp.Game.StateVersion != ver+1 {
		t.Fatalf("state_version: want %d, got %d", ver+1, resp.Game.StateVersion)
	}
	if resp.Move.FENBefore == resp.Move.FENAfter {
		t.Fatal("FEN should change after a legal move")
	}
	if len(resp.Game.MoveHistory) != 1 {
		t.Fatalf("expected 1 move in history, got %d", len(resp.Game.MoveHistory))
	}
	if resp.Game.MoveHistory[0].UCI != "e2e4" {
		t.Fatalf("history[0].uci: want e2e4, got %q", resp.Game.MoveHistory[0].UCI)
	}
}

func TestSubmitMove_IllegalMove(t *testing.T) {
	h := newTestServer(t)
	clientID := uuid.New().String()
	gameID, _ := getNextGame(t, h, clientID)

	rec := doRequest(t, h, http.MethodPost, "/api/v1/games/"+gameID+"/moves",
		map[string]any{"uci": "e2e5", "expected_version": 0},
		map[string]string{"X-Client-Id": clientID},
	)
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
	clientID := uuid.New().String()
	gameID, _ := getNextGame(t, h, clientID)

	rec := doRequest(t, h, http.MethodPost, "/api/v1/games/"+gameID+"/moves",
		map[string]any{"uci": "zzzz", "expected_version": 0},
		map[string]string{"X-Client-Id": clientID},
	)
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
	clientID := uuid.New().String()
	gameID, _ := getNextGame(t, h, clientID)

	rec := doRequest(t, h, http.MethodPost, "/api/v1/games/"+gameID+"/moves",
		map[string]any{"uci": "e2e4", "expected_version": 99},
		map[string]string{"X-Client-Id": clientID},
	)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ── New tests ─────────────────────────────────────────────────────────────────

func TestGetNext_RequiresClientID(t *testing.T) {
	h := newTestServer(t)
	rec := doRequest(t, h, http.MethodGet, "/api/v1/games/next", nil, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestGetNext_ReturnsGameWithHistory(t *testing.T) {
	h := newTestServer(t)
	clientID := uuid.New().String()
	rec := doRequest(t, h, http.MethodGet, "/api/v1/games/next", nil, map[string]string{
		"X-Client-Id": clientID,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Game struct {
			GameID      string        `json:"game_id"`
			Status      string        `json:"status"`
			MoveHistory []interface{} `json:"move_history"`
		} `json:"game"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Game.GameID == "" {
		t.Fatal("expected non-empty game_id")
	}
	if resp.Game.Status != "ongoing" {
		t.Fatalf("expected status ongoing, got %q", resp.Game.Status)
	}
	if resp.Game.MoveHistory == nil {
		t.Fatal("move_history must not be null")
	}
}

// TestGetNext_NeverRepeatsSameClient: same client should get different games.
func TestGetNext_NeverRepeatsSameClient(t *testing.T) {
	// Use a store with 2 games so we can exhaust and reassign.
	store := memory.New(2)
	h := newTestServerWithStore(t, store)
	clientID := uuid.New().String()

	id1, _ := getNextGame(t, h, clientID)
	id2, _ := getNextGame(t, h, clientID)

	if id1 == id2 {
		t.Fatalf("same client received the same game twice: %s", id1)
	}
}

// TestGetNext_BatchCreationOnEmptyStore: when store has 0 games, a batch is
// created automatically and the client receives a game.
func TestGetNext_BatchCreationOnEmptyStore(t *testing.T) {
	store := memory.New(0) // start with no games
	h := newTestServerWithStore(t, store)
	clientID := uuid.New().String()

	rec := doRequest(t, h, http.MethodGet, "/api/v1/games/next", nil, map[string]string{
		"X-Client-Id": clientID,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 after batch creation, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Game struct {
			GameID string `json:"game_id"`
		} `json:"game"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Game.GameID == "" {
		t.Fatal("expected a game after batch creation")
	}
}

// TestSubmitMove_OneMoveLimit: a client cannot submit a second move in the same game.
func TestSubmitMove_OneMoveLimit(t *testing.T) {
	h := newTestServer(t)
	clientID := uuid.New().String()
	gameID, ver := getNextGame(t, h, clientID)

	// First move — must succeed.
	rec := doRequest(t, h, http.MethodPost, "/api/v1/games/"+gameID+"/moves",
		map[string]any{"uci": "e2e4", "expected_version": ver},
		map[string]string{"X-Client-Id": clientID},
	)
	if rec.Code != http.StatusOK {
		t.Fatalf("first move: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Second move — must be rejected even if it's legal in chess terms.
	// After e2e4 it's black's turn; e7e5 is a legal black reply, but the
	// client has already used their one-move allowance.
	rec = doRequest(t, h, http.MethodPost, "/api/v1/games/"+gameID+"/moves",
		map[string]any{"uci": "e7e5", "expected_version": ver + 1},
		map[string]string{"X-Client-Id": clientID},
	)
	if rec.Code != http.StatusConflict {
		t.Fatalf("second move: expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Code != "one_move_limit" {
		t.Fatalf("expected code one_move_limit, got %q", resp.Code)
	}
}

// TestSubmitMove_NotAssigned: submit without claiming via /games/next first → 403.
func TestSubmitMove_NotAssigned(t *testing.T) {
	// Use store with pre-seeded games.
	store := memory.New(1)
	h := newTestServerWithStore(t, store)

	// Get a game ID via /games/assigned (legacy, no tracking).
	rec := doRequest(t, h, http.MethodGet, "/api/v1/games/assigned", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("assigned: expected 200, got %d", rec.Code)
	}
	var assignResp struct {
		Game struct{ GameID string `json:"game_id"` } `json:"game"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&assignResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	gameID := assignResp.Game.GameID

	// Submit move with a clientID that was never assigned via /games/next.
	unassignedClient := uuid.New().String()
	rec = doRequest(t, h, http.MethodPost, "/api/v1/games/"+gameID+"/moves",
		map[string]any{"uci": "e2e4", "expected_version": 0},
		map[string]string{"X-Client-Id": unassignedClient},
	)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestSubmitMove_FromToForm: accepts from/to/promotion instead of uci field.
func TestSubmitMove_FromToForm(t *testing.T) {
	h := newTestServer(t)
	clientID := uuid.New().String()
	gameID, ver := getNextGame(t, h, clientID)

	rec := doRequest(t, h, http.MethodPost, "/api/v1/games/"+gameID+"/moves",
		map[string]any{"from": "e2", "to": "e4", "expected_version": ver},
		map[string]string{"X-Client-Id": clientID},
	)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Move struct{ UCI string `json:"uci"` } `json:"move"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Move.UCI != "e2e4" {
		t.Fatalf("expected uci e2e4, got %q", resp.Move.UCI)
	}
}

// TestGetGame_IncludesMoveHistory: GET /games/:id returns move_history.
func TestGetGame_IncludesMoveHistory(t *testing.T) {
	h := newTestServer(t)
	clientID := uuid.New().String()
	gameID, ver := getNextGame(t, h, clientID)

	// Submit a move to populate history.
	doRequest(t, h, http.MethodPost, "/api/v1/games/"+gameID+"/moves",
		map[string]any{"uci": "e2e4", "expected_version": ver},
		map[string]string{"X-Client-Id": clientID},
	)

	rec := doRequest(t, h, http.MethodGet, "/api/v1/games/"+gameID, nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		MoveHistory []struct {
			Ply int    `json:"ply"`
			UCI string `json:"uci"`
		} `json:"move_history"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.MoveHistory) != 1 {
		t.Fatalf("expected 1 move in history, got %d", len(resp.MoveHistory))
	}
	if resp.MoveHistory[0].Ply != 0 {
		t.Fatalf("expected ply 0, got %d", resp.MoveHistory[0].Ply)
	}
}

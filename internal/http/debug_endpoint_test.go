package httpx

// Tests for GET /debug/players.
//
// TestDebugPlayers_Empty verifies that an empty world returns an empty JSON array.
// TestDebugPlayers_WithPlayers verifies that players' WheelsOff values appear
// in the response and that playerIDs are reported correctly.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/JoakimCarlsson/sim-racing/internal/physics"
	"github.com/JoakimCarlsson/sim-racing/internal/protocol"
	"github.com/JoakimCarlsson/sim-racing/internal/sim"
)

// nullInputSource is a minimal sim.InputSource for tests in this package.
type nullInputSource struct{}

func (n *nullInputSource) DrainInputs(_ []protocol.ClientInput) int { return 0 }

// addSimPlayer creates a minimal PlayerSim and adds it to the world.
// It returns the PlayerSim so callers can patch fields after insertion.
func addSimPlayer(
	t *testing.T,
	w *sim.World,
	id sim.PlayerID,
	wheelsOff uint8,
) *sim.PlayerSim {
	t.Helper()
	p := &sim.PlayerSim{
		ID:        id,
		Source:    &nullInputSource{},
		Constants: physics.DefaultConstants,
		WheelsOff: wheelsOff,
		State: physics.State{
			Orientation: [4]float32{0, 0, 0, 1},
		},
	}
	w.AddPlayer(p)
	return p
}

// TestDebugPlayers_Empty verifies that GET /debug/players returns an empty
// JSON array when no players are connected.
func TestDebugPlayers_Empty(t *testing.T) {
	w := sim.New()
	deps := Deps{
		StartedAt: time.Now(),
		Version:   "test",
		World:     w,
	}
	handler := NewHandler(deps)

	req := httptest.NewRequest(http.MethodGet, "/debug/players", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var players []playerDebugDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &players); err != nil {
		t.Fatalf("unmarshal: %v (body=%s)", err, rec.Body.String())
	}
	if len(players) != 0 {
		t.Errorf("expected empty array, got %d players", len(players))
	}
}

// TestDebugPlayers_WithPlayers verifies that WheelsOff is reported per player
// and that playerIDs appear correctly.
func TestDebugPlayers_WithPlayers(t *testing.T) {
	w := sim.New()

	// Player 1 with WheelsOff=0.
	addSimPlayer(t, w, 1, 0)
	// Player 2: inject WheelsOff=2 directly after adding.
	p2 := addSimPlayer(t, w, 2, 0)
	p2.WheelsOff = 2

	deps := Deps{
		StartedAt: time.Now(),
		Version:   "test",
		World:     w,
	}
	handler := NewHandler(deps)

	req := httptest.NewRequest(http.MethodGet, "/debug/players", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", rec.Code, rec.Body.String())
	}

	var players []playerDebugDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &players); err != nil {
		t.Fatalf("unmarshal: %v (body=%s)", err, rec.Body.String())
	}
	if len(players) != 2 {
		t.Fatalf("expected 2 players, got %d", len(players))
	}

	byID := make(map[uint16]playerDebugDTO)
	for _, pd := range players {
		byID[pd.PlayerID] = pd
	}

	if p1, ok := byID[1]; !ok {
		t.Error("player 1 missing from response")
	} else if p1.WheelsOff != 0 {
		t.Errorf("player 1: WheelsOff=%d, want 0", p1.WheelsOff)
	}

	if p2dto, ok := byID[2]; !ok {
		t.Error("player 2 missing from response")
	} else if p2dto.WheelsOff != 2 {
		t.Errorf("player 2: WheelsOff=%d, want 2", p2dto.WheelsOff)
	}
}

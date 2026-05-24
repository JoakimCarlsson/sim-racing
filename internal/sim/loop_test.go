package sim

import (
	"context"
	"go/build"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JoakimCarlsson/sim-racing/internal/physics"
	"github.com/JoakimCarlsson/sim-racing/internal/protocol"
)

// ---- AC5: internal/sim must not import internal/realtime ----

// TestNoRealtimeImport verifies that no .go file in internal/sim/ imports
// internal/realtime. The InputSource interface must be used instead.
func TestNoRealtimeImport(t *testing.T) {
	t.Helper()
	// Use go/build to get the list of .go files in this package.
	pkg, err := build.ImportDir(filepath.Join("."), 0)
	if err != nil {
		t.Fatalf("build.ImportDir: %v", err)
	}

	const forbidden = "github.com/JoakimCarlsson/sim-racing/internal/realtime"
	for _, imp := range pkg.Imports {
		if strings.Contains(imp, forbidden) || imp == forbidden {
			t.Errorf(
				"internal/sim must not import internal/realtime; found import %q",
				imp,
			)
		}
	}
	// Also check test imports (TestImports + XTestImports).
	allTestImports := append(
		pkg.TestImports,
		pkg.XTestImports...) //nolint:gocritic
	for _, imp := range allTestImports {
		if imp == forbidden || strings.Contains(imp, forbidden) {
			t.Errorf(
				"internal/sim test files must not import internal/realtime; found %q",
				imp,
			)
		}
	}
}

// ---- helpers ----

// staticSource is a test InputSource that replays a fixed slice of inputs.
// Each DrainInputs call returns all remaining inputs (up to len(dst)) in
// FIFO order, removing them from the queue.
type staticSource struct {
	inputs []protocol.ClientInput
	pos    int
}

func (s *staticSource) DrainInputs(dst []protocol.ClientInput) int {
	n := copy(dst, s.inputs[s.pos:])
	s.pos += n
	return n
}

// emptySource always returns 0 inputs (simulates idle connection).
type emptySource struct{}

func (e *emptySource) DrainInputs(_ []protocol.ClientInput) int { return 0 }

// ---- AC1: 60 Hz tick rate over 1.5 s window ±5% ----

func TestTickRate60Hz(t *testing.T) {
	w := New()
	// No players required; we just need the ticker to fire.

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var ticks atomic.Int64
	// Wrap the world tick so we can count fires without changing loop.go.
	// We run the real Run but intercept by adding a dummy player whose
	// DrainInputs we use to count calls.
	counter := &tickCounter{ch: make(chan struct{}, 1024)}
	p := &PlayerSim{
		ID:        1,
		Source:    counter,
		Constants: physics.DefaultConstants,
	}
	w.AddPlayer(p)

	go w.Run(ctx)

	// Measure over exactly 1.5 seconds.
	measureWindow := 1500 * time.Millisecond
	time.Sleep(50 * time.Millisecond) // let the loop start
	counter.reset()
	start := time.Now()
	time.Sleep(measureWindow)
	elapsed := time.Since(start)
	count := counter.count()

	// expected ticks at 60 Hz over elapsed
	expected := float64(elapsed) / float64(time.Second) * 60.0
	ratio := float64(count) / expected

	t.Logf(
		"ticks=%d expected=%.1f ratio=%.3f elapsed=%s",
		count,
		expected,
		ratio,
		elapsed,
	)

	_ = ticks.Load()

	const tolerance = 0.05
	if ratio < 1-tolerance || ratio > 1+tolerance {
		t.Errorf(
			"tick rate out of ±5%% tolerance: got ratio %.3f (ticks=%d expected=%.1f)",
			ratio,
			count,
			expected,
		)
	}
}

// tickCounter is an InputSource that counts how many times DrainInputs is
// called. It is used as a proxy to count tick fires in AC1.
type tickCounter struct {
	ch    chan struct{}
	n     atomic.Int64
	armed atomic.Bool
}

func (tc *tickCounter) reset() {
	tc.n.Store(0)
	tc.armed.Store(true)
}

func (tc *tickCounter) count() int64 {
	return tc.n.Load()
}

func (tc *tickCounter) DrainInputs(dst []protocol.ClientInput) int {
	if tc.armed.Load() {
		tc.n.Add(1)
	}
	return 0
}

// ---- AC2: inputs applied in sequential order ----

func TestInputsAppliedInSequentialOrder(t *testing.T) {
	w := New()

	// Build 6 inputs with increasing Seq. seq=1..6, all full throttle gear 2.
	var inputs []protocol.ClientInput
	for i := uint32(1); i <= 6; i++ {
		inputs = append(inputs, protocol.ClientInput{
			Seq:   i,
			Input: physics.Input{Throttle: 1.0, Gear: 2},
		})
	}

	src := &staticSource{inputs: inputs}
	p := &PlayerSim{
		ID:        1,
		Source:    src,
		Constants: physics.DefaultConstants,
	}
	w.AddPlayer(p)

	dt := float32(1.0 / 60.0)
	// One tick — should drain and apply all 6 inputs in order (maxCatchupTicks=4
	// caps at 4). After the first tick, LastAppliedSeq must be 4 (the cap).
	w.tick(dt)
	if p.LastAppliedSeq != 4 {
		t.Errorf(
			"expected LastAppliedSeq=4 (catch-up cap), got %d",
			p.LastAppliedSeq,
		)
	}

	// Remaining inputs (5, 6) are already consumed (DrainInputs is non-restoring).
	// Verify the state advanced (position should have moved forward slightly
	// due to throttle in gear 2).
	if p.State.RPM == 0 && p.State.LinearVel[2] == 0 {
		t.Error("physics did not advance state after applying inputs")
	}

	// Now add inputs with lower seq — they must be skipped.
	p.LastAppliedSeq = 10
	oldState := p.State
	staleInputs := []protocol.ClientInput{
		{Seq: 5, Input: physics.Input{Throttle: 1.0, Gear: 2}},
		{Seq: 8, Input: physics.Input{Throttle: 1.0, Gear: 2}},
	}
	src2 := &staticSource{inputs: staleInputs}
	p.Source = src2
	w.tick(dt)
	// All inputs have seq <= 10; they should all be skipped. With no inputs
	// applied the loop applies a zero-input step (coast).
	if p.LastAppliedSeq != 10 {
		t.Errorf(
			"stale inputs must not advance LastAppliedSeq; got %d",
			p.LastAppliedSeq,
		)
	}
	// State should differ from oldState because a coast step was applied.
	if p.State == oldState {
		t.Error("expected coast step to modify state even with no valid inputs")
	}
}

// ---- AC3: disconnect removes the PlayerSim ----

func TestDisconnectRemovesPlayer(t *testing.T) {
	w := New()

	p := &PlayerSim{
		ID:        42,
		Source:    &emptySource{},
		Constants: physics.DefaultConstants,
	}
	w.AddPlayer(p)

	if got := w.Get(42); got == nil {
		t.Fatal("player 42 not found after AddPlayer")
	}

	w.RemovePlayer(42)

	if got := w.Get(42); got != nil {
		t.Error("player 42 still present after RemovePlayer")
	}

	// Ensure a subsequent tick with no players does not panic.
	w.tick(float32(1.0 / 60.0))
}

// ---- AC4: 600-tick determinism; Position[2] advances >5 m ----

func TestDeterminism600Ticks(t *testing.T) {
	dt := float32(1.0 / 60.0)

	// Build the same initial state and identical input stream twice, then
	// compare final state byte-for-byte.

	makeState := func() (*PlayerSim, *staticSource) {
		var inputs []protocol.ClientInput
		for i := uint32(1); i <= 600; i++ {
			// Simulate shifting: gear 1 for first 100, gear 2 for next 200, gear 3
			// for remaining 300.
			g := int8(1)
			if i > 100 {
				g = 2
			}
			if i > 300 {
				g = 3
			}
			inputs = append(inputs, protocol.ClientInput{
				Seq:   i,
				Input: physics.Input{Throttle: 1.0, Gear: g},
			})
		}
		src := &staticSource{inputs: inputs}
		p := &PlayerSim{
			ID:        1,
			Source:    src,
			Constants: physics.DefaultConstants,
		}
		return p, src
	}

	// Run A.
	pA, _ := makeState()
	wA := New()
	wA.AddPlayer(pA)
	for i := 0; i < 600; i++ {
		wA.tick(dt)
	}
	stateA := pA.State

	// Run B (independent).
	pB, _ := makeState()
	wB := New()
	wB.AddPlayer(pB)
	for i := 0; i < 600; i++ {
		wB.tick(dt)
	}
	stateB := pB.State

	if stateA != stateB {
		t.Errorf(
			"determinism violated:\n  run A Position=%v\n  run B Position=%v",
			stateA.Position,
			stateB.Position,
		)
	}

	// Position[2] is the forward axis; verify it advanced more than 5 m.
	// With full throttle for 600 ticks at 60 Hz (= 10 s) the car should have
	// travelled well beyond 5 m.
	if stateA.Position[2] <= 5.0 {
		t.Errorf(
			"expected Position[2] > 5 m after 600 ticks, got %.3f",
			stateA.Position[2],
		)
	}
	t.Logf("Position[2] after 600 ticks: %.3f m", stateA.Position[2])
}

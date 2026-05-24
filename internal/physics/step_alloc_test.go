package physics_test

// Zero-allocation test for physics.Step (AC3).

import (
	"testing"

	"github.com/JoakimCarlsson/sim-racing/internal/physics"
)

// TestStepZeroAllocs asserts that physics.Step produces no heap allocations
// per call (AC3). A single allocation invalidates the WASM determinism model
// because GC pauses introduce non-determinism in real-time framerate contexts.
func TestStepZeroAllocs(t *testing.T) {
	c := physics.DefaultConstants
	s := physics.State{
		LinearVel: [3]float32{0, 0, 0},
		Gear:      1,
	}
	in := physics.Input{
		Throttle: 0.5,
		Brake:    0.0,
		Gear:     1,
	}
	dt := float32(tickDT)

	allocs := testing.AllocsPerRun(1000, func() {
		s = physics.Step(s, in, c, dt)
	})

	if allocs != 0 {
		t.Errorf("Step allocates %.0f heap objects per call, want 0", allocs)
	}
}

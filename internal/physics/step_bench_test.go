package physics_test

// Benchmark for physics.Step (AC3).

import (
	"testing"

	"github.com/JoakimCarlsson/sim-racing/internal/physics"
)

// BenchmarkStep measures the throughput of a single physics.Step call.
// Run with -benchmem to confirm zero allocations (AC3).
func BenchmarkStep(b *testing.B) {
	c := physics.DefaultConstants
	s := physics.State{
		Position:  [3]float32{0, suspGroundY, 0},
		LinearVel: [3]float32{0, 0, 20},
		Gear:      3,
	}
	in := physics.Input{
		Throttle: 0.8,
		Brake:    0.0,
		Gear:     3,
	}
	dt := float32(tickDT)
	sampler := physics.FlatGround(0)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s = physics.Step(s, in, c, sampler, dt)
	}
	// Use s to prevent the compiler from eliding the call.
	_ = s
}

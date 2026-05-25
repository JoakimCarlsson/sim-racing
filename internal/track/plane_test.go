package track_test

import (
	"testing"

	"github.com/JoakimCarlsson/sim-racing/internal/track"
)

// TestPlane_Crossed exercises all documented boundary semantics for
// Plane.Crossed(prev, cur Vec3).
//
// The canonical plane used in most sub-tests:
//
//	P0 = (-5, 0, 0), P1 = (5, 0, 0), Normal = (0, 0, 1)
//
// The gate runs along the X-axis at Z=0.  A vehicle crossing from negative Z
// toward positive Z is a "forward" crossing (signedPrev < 0, signedCur > 0).
func TestPlane_Crossed(t *testing.T) {
	t.Parallel()

	// gate on Z=0 plane, gate width from x=-5 to x=5
	gate := track.Plane{
		P0:     track.Vec3{-5, 0, 0},
		P1:     track.Vec3{5, 0, 0},
		Normal: track.Vec3{0, 0, 1},
	}

	cases := []struct {
		name        string
		plane       track.Plane
		prev        track.Vec3
		cur         track.Vec3
		wantCrossed bool
		// wantT is only checked when wantCrossed==true
		wantT    float32
		checkT   bool
		wantTMin float32 // inclusive lower bound (used instead of exact when checkT==false)
		wantTMax float32 // inclusive upper bound
	}{
		// AC1 — forward and reverse crossings
		{
			name:        "forward_crossing",
			plane:       gate,
			prev:        track.Vec3{0, 0, -1},
			cur:         track.Vec3{0, 0, 1},
			wantCrossed: true,
			checkT:      true,
			wantT:       0.5,
		},
		{
			name:        "reverse_crossing",
			plane:       gate,
			prev:        track.Vec3{0, 0, 1},
			cur:         track.Vec3{0, 0, -1},
			wantCrossed: true,
			checkT:      true,
			wantT:       0.5,
		},

		// AC2 — high speed / straddle over multiple metres in one tick
		{
			// Car jumps from Z=-10 to Z=+10 in one tick; must still detect.
			name:        "high_speed_one_tick",
			plane:       gate,
			prev:        track.Vec3{0, 0, -10},
			cur:         track.Vec3{0, 0, 10},
			wantCrossed: true,
			checkT:      true,
			wantT:       0.5,
		},
		{
			// Plane is not axis-aligned; same straddle logic must work.
			name: "off_axis_plane",
			plane: track.Plane{
				P0:     track.Vec3{0, 0, -5},
				P1:     track.Vec3{0, 0, 5},
				Normal: track.Vec3{1, 0, 0}, // X=0 plane, normal pointing +X
			},
			prev:        track.Vec3{-3, 0, 0},
			cur:         track.Vec3{3, 0, 0},
			wantCrossed: true,
			checkT:      true,
			wantT:       0.5,
		},

		// AC3 — near-miss, glancing, exactly-on-plane, lateral miss
		{
			// Both endpoints on same side of the plane → no crossing.
			name:        "near_miss_parallel",
			plane:       gate,
			prev:        track.Vec3{0, 0, -0.001},
			cur:         track.Vec3{0, 0, -0.0005},
			wantCrossed: false,
		},
		{
			// cur is exactly on the plane (signedCur==0), prev != 0 → crossing at t=1.
			name:        "glancing_cur_on_plane",
			plane:       gate,
			prev:        track.Vec3{0, 0, -1},
			cur:         track.Vec3{0, 0, 0},
			wantCrossed: true,
			checkT:      true,
			wantT:       1.0,
		},
		{
			// prev is exactly on the plane (signedPrev==0) → NOT a crossing
			// (boundary policy: avoids double-count on next tick).
			name:        "exactly_on_plane_prev",
			plane:       gate,
			prev:        track.Vec3{0, 0, 0},
			cur:         track.Vec3{0, 0, 1},
			wantCrossed: false,
		},
		{
			// Motion crosses the plane's Z=0 but the gate only covers x ∈ [-5,5].
			// X=8 is outside the gate → lateral miss → no crossing.
			name:        "lateral_miss",
			plane:       gate,
			prev:        track.Vec3{8, 0, -1},
			cur:         track.Vec3{8, 0, 1},
			wantCrossed: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			crossed, tVal := tc.plane.Crossed(tc.prev, tc.cur)
			if crossed != tc.wantCrossed {
				t.Errorf(
					"Crossed(%v, %v) crossed=%v, want %v",
					tc.prev, tc.cur, crossed, tc.wantCrossed,
				)
			}
			if tc.wantCrossed && tc.checkT {
				const eps = 1e-5
				diff := tVal - tc.wantT
				if diff < -eps || diff > eps {
					t.Errorf(
						"Crossed(%v, %v) t=%v, want %v (±%v)",
						tc.prev, tc.cur, tVal, tc.wantT, eps,
					)
				}
			}
		})
	}
}

// TestPlane_Crossed_NoAlloc asserts the hot path allocates zero bytes.
// This satisfies the zero-alloc requirement in non_goals.
func TestPlane_Crossed_NoAlloc(t *testing.T) {
	gate := track.Plane{
		P0:     track.Vec3{-5, 0, 0},
		P1:     track.Vec3{5, 0, 0},
		Normal: track.Vec3{0, 0, 1},
	}
	prev := track.Vec3{0, 0, -1}
	cur := track.Vec3{0, 0, 1}

	allocs := testing.AllocsPerRun(1000, func() {
		_, _ = gate.Crossed(prev, cur)
	})
	if allocs != 0 {
		t.Errorf("Crossed allocates %.0f allocs/op, want 0", allocs)
	}
}

package physics_test

// Tests for WheelXZ — the exported wheel contact-patch helper.
//
// AC-mapping: internal/physics/wheels_test.go exercises the refactored
// WheelXZ function that step.go delegates to.
//
// Key property: the returned positions must be byte-identical to the
// wheel positions that step.go inlined before the refactor; the
// TestReplayDeterminism golden must therefore not change.

import (
	"math"
	"testing"

	"github.com/JoakimCarlsson/sim-racing/internal/physics"
)

// TestWheelXZ_IdentityOrientation checks that with zero yaw (identity
// quaternion [0,0,0,1]) the wheel XZ positions are the CoM XZ plus
// the body-frame half-track / half-wheelbase offsets.
func TestWheelXZ_IdentityOrientation(t *testing.T) {
	c := physics.DefaultConstants

	s := physics.State{
		Position:    [3]float32{10, 1, 5},
		Orientation: [4]float32{0, 0, 0, 1}, // identity — no yaw
	}

	pts := physics.WheelXZ(s, c)

	halfTrack := c.TrackWidth / 2
	halfWB := c.Wheelbase / 2

	// Expected positions (body offsets in X, Z; world offset is the same when
	// yaw = 0):
	//   FL: (+halfTrack, +halfWB)
	//   FR: (-halfTrack, +halfWB)
	//   RL: (+halfTrack, -halfWB)
	//   RR: (-halfTrack, -halfWB)
	want := [4][2]float32{
		{s.Position[0] + halfTrack, s.Position[2] + halfWB}, // FL
		{s.Position[0] - halfTrack, s.Position[2] + halfWB}, // FR
		{s.Position[0] + halfTrack, s.Position[2] - halfWB}, // RL
		{s.Position[0] - halfTrack, s.Position[2] - halfWB}, // RR
	}

	const tol = float32(1e-5)
	for i, w := range want {
		if diff := abs32f(pts[i][0] - w[0]); diff > tol {
			t.Errorf(
				"wheel %d X: got %v want %v (diff %v)",
				i,
				pts[i][0],
				w[0],
				diff,
			)
		}
		if diff := abs32f(pts[i][1] - w[1]); diff > tol {
			t.Errorf(
				"wheel %d Z: got %v want %v (diff %v)",
				i,
				pts[i][1],
				w[1],
				diff,
			)
		}
	}
}

// TestWheelXZ_90DegYaw checks that a 90-degree left yaw rotates the
// body-frame X axis to align with the world -Z axis.
// A 90° yaw quaternion about Y is [0, sin(45°), 0, cos(45°)].
func TestWheelXZ_90DegYaw(t *testing.T) {
	c := physics.DefaultConstants

	sin45 := float32(math.Sin(math.Pi / 4))
	cos45 := float32(math.Cos(math.Pi / 4))

	s := physics.State{
		Position:    [3]float32{0, 1, 0},
		Orientation: [4]float32{0, sin45, 0, cos45}, // 90° yaw (CCW)
	}

	pts := physics.WheelXZ(s, c)

	// With 90° yaw, body +X maps to world +Z, body +Z maps to world -X.
	// WheelXZ uses sinYaw = 2*qw*qy, cosYaw = 1 - 2*qy².
	qy := sin45
	qw := cos45
	sinYaw := 2 * qw * qy
	cosYaw := 1 - 2*qy*qy

	halfTrack := c.TrackWidth / 2
	halfWB := c.Wheelbase / 2

	rotXZ := func(bx, bz float32) (float32, float32) {
		wx := cosYaw*bx - sinYaw*bz
		wz := sinYaw*bx + cosYaw*bz
		return wx, wz
	}

	offsets := [4][2]float32{
		{+halfTrack, +halfWB},
		{-halfTrack, +halfWB},
		{+halfTrack, -halfWB},
		{-halfTrack, -halfWB},
	}

	const tol = float32(1e-5)
	for i, off := range offsets {
		expX, expZ := rotXZ(off[0], off[1])
		expX += s.Position[0]
		expZ += s.Position[2]
		if diff := abs32f(pts[i][0] - expX); diff > tol {
			t.Errorf("wheel %d X: got %v want %v", i, pts[i][0], expX)
		}
		if diff := abs32f(pts[i][1] - expZ); diff > tol {
			t.Errorf("wheel %d Z: got %v want %v", i, pts[i][1], expZ)
		}
	}
}

// TestWheelXZ_ZeroConstants verifies WheelXZ handles zero Wheelbase/TrackWidth
// gracefully (all four wheels coincide with CoM XZ).
func TestWheelXZ_ZeroConstants(t *testing.T) {
	c := physics.Constants{} // zero — no wheelbase or track width

	s := physics.State{
		Position:    [3]float32{3, 0, 7},
		Orientation: [4]float32{0, 0, 0, 1},
	}

	pts := physics.WheelXZ(s, c)

	const tol = float32(1e-6)
	for i, p := range pts {
		if diff := abs32f(p[0] - s.Position[0]); diff > tol {
			t.Errorf("wheel %d X: got %v want %v", i, p[0], s.Position[0])
		}
		if diff := abs32f(p[1] - s.Position[2]); diff > tol {
			t.Errorf("wheel %d Z: got %v want %v", i, p[1], s.Position[2])
		}
	}
}

func abs32f(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

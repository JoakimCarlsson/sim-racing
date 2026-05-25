package physics

// WheelXZ returns the world-space (X, Z) contact-patch positions for all four
// wheels given the vehicle state and constants.
//
// Wheel order: FL=0, FR=1, RL=2, RR=3.
// The body-frame offsets are:
//
//	FL: (+halfTrack, +halfWB)   FR: (-halfTrack, +halfWB)
//	RL: (+halfTrack, -halfWB)   RR: (-halfTrack, -halfWB)
//
// The orientation quaternion is assumed to encode yaw only (the rest of the
// physics model uses a single-track approximation). For a yaw-only quaternion
// [0, qy, 0, qw]:
//
//	sinYaw = 2 * qw * qy
//	cosYaw = 1 - 2 * qy²
//
// This function is pure and allocation-free; it can be called from WASM.
// It is the single authoritative wheel-XZ computation; step.go delegates to it
// so the same formula cannot drift.
func WheelXZ(s State, c Constants) [4][2]float32 {
	halfTrack := c.TrackWidth / 2.0
	halfWB := c.Wheelbase / 2.0

	// Extract sinYaw / cosYaw from the yaw-only quaternion component.
	qy := s.Orientation[1]
	qw := s.Orientation[3]
	sinYaw := 2 * qw * qy
	cosYaw := 1 - 2*qy*qy

	// rotXZ rotates a body-frame (bx, bz) offset into world-frame (wx, wz).
	rotXZ := func(bx, bz float32) (float32, float32) {
		wx := cosYaw*bx - sinYaw*bz
		wz := sinYaw*bx + cosYaw*bz
		return wx, wz
	}

	// Body-frame offsets: [bx, bz] per wheel (FL, FR, RL, RR).
	type off [2]float32
	offsets := [4]off{
		{+halfTrack, +halfWB}, // FL
		{-halfTrack, +halfWB}, // FR
		{+halfTrack, -halfWB}, // RL
		{-halfTrack, -halfWB}, // RR
	}

	var out [4][2]float32
	for i, o := range offsets {
		wx, wz := rotXZ(o[0], o[1])
		out[i][0] = s.Position[0] + wx
		out[i][1] = s.Position[2] + wz
	}
	return out
}

package track

// Crossed reports whether the segment from prev to cur crosses the plane's
// gate, and if so, returns the fractional position t ∈ (0, 1] along the
// segment at which the crossing occurs.
//
// Algorithm — XZ-projection ray-cast with lateral gate check:
//
//  1. Project the plane into the XZ plane (Y is ignored, gates are on Y=0).
//  2. Compute the signed distance of prev and cur from the infinite plane
//     defined by the gate normal and its centroid.
//  3. A crossing is detected when signedPrev and signedCur are on opposite
//     sides.  The fractional position t = signedPrev / (signedPrev -
//     signedCur).
//  4. Interpolate the crossing point and check that it lies within the
//     segment [P0, P1] (lateral gate check via parameter u ∈ [0, 1]).
//
// Boundary policy (deterministic, avoids double-count):
//   - signedPrev == 0 → not a crossing (the tick that moved the car onto the
//     plane has already been counted; treating the next tick as a fresh
//     crossing would double-fire).
//   - signedCur == 0 with signedPrev != 0 → crossing at t = 1.0 (the car
//     arrives exactly on the plane this tick; count it once, here).
//
// The caller can infer traversal direction from sign(dot(motion, p.Normal)):
// positive → forward (same direction as Normal), negative → reverse.
//
// This function is zero-alloc on the hot path.
func (p Plane) Crossed(prev, cur Vec3) (crossed bool, t float32) {
	// Gate centroid used as the plane's reference point (any point on the
	// plane works; the centroid is numerically balanced).
	cx := (p.P0[0] + p.P1[0]) * 0.5
	cz := (p.P0[2] + p.P1[2]) * 0.5

	nx := p.Normal[0]
	nz := p.Normal[2]

	// Signed distance = dot((pos - centroid), normal)  (XZ projection only).
	sPrev := (prev[0]-cx)*nx + (prev[2]-cz)*nz
	sCur := (cur[0]-cx)*nx + (cur[2]-cz)*nz

	// Boundary policy: prev exactly on the plane → not a crossing.
	if sPrev == 0 {
		return false, 0
	}

	// Same side → no crossing.
	if (sPrev > 0 && sCur > 0) || (sPrev < 0 && sCur < 0) {
		return false, 0
	}

	// sCur == 0 with sPrev != 0 → crossing at t=1.
	if sCur == 0 {
		t = 1.0
	} else {
		// General case: linear interpolation.
		t = sPrev / (sPrev - sCur)
	}

	// Compute the XZ crossing point.
	hitX := prev[0] + t*(cur[0]-prev[0])
	hitZ := prev[2] + t*(cur[2]-prev[2])

	// Lateral gate check: project hitX/hitZ onto the gate segment and
	// verify that the parameter u is in [0, 1].
	//
	// Gate vector g = P1 - P0 (XZ only).
	gx := p.P1[0] - p.P0[0]
	gz := p.P1[2] - p.P0[2]
	gLen2 := gx*gx + gz*gz

	if gLen2 == 0 {
		// Degenerate gate (P0 == P1): accept only if the hit is exactly at P0.
		if hitX == p.P0[0] && hitZ == p.P0[2] {
			return true, t
		}
		return false, 0
	}

	// u = dot(hit - P0, g) / |g|^2
	u := ((hitX-p.P0[0])*gx + (hitZ-p.P0[2])*gz) / gLen2
	if u < 0 || u > 1 {
		return false, 0
	}

	return true, t
}

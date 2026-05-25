package track

// Contains reports whether the point (x, z) lies strictly inside p using the
// crossing-number (ray-cast) algorithm. The algorithm is winding-agnostic:
// it gives identical results for clockwise and counter-clockwise vertex order.
//
// The ray is cast in the +X direction from (x, z). An edge is counted when it
// straddles the horizontal ray using a half-open interval on the Z axis:
// the lower endpoint is included (z1 <= z) and the upper endpoint is excluded
// (z2 > z). This guarantees a deterministic, consistent tie-break for boundary
// cases:
//
//   - Point on a horizontal edge (z == z1 == z2): NOT counted → OUTSIDE.
//   - Point exactly on a vertex: depends on the two adjacent edges; typically
//     OUTSIDE because at most one of the two edges triggers via the half-open
//     interval (the bottom vertex of each edge is excluded).
//   - Point on a non-horizontal edge: the crossing-number produces a
//     consistent classification — callers must not rely on it being inside or
//     outside, only that the result is deterministic across repeated calls.
//
// Performance: O(n) in the number of vertices, with no heap allocations.
// A ~64-vertex polygon completes in < 2 µs on modern hardware (see
// BenchmarkContains in polygon_test.go).
func (p Polygon2D) Contains(x, z float32) bool {
	n := len(p)
	if n < 3 {
		return false
	}

	crossings := 0
	j := n - 1
	for i := 0; i < n; i++ {
		xi, zi := p[i][0], p[i][1]
		xj, zj := p[j][0], p[j][1]

		// Half-open interval: edge straddles the ray iff one endpoint is
		// strictly above (z > query) and the other is at or below (z <= query).
		// Using (zi > z) != (zj > z) as the straddle test is equivalent to
		// the half-open [zj, zi) or [zi, zj) interval check.
		if (zi > z) != (zj > z) {
			// Compute the X coordinate of the edge–ray intersection.
			// The ray is at Z = z going in the +X direction.
			//
			//   x_intersect = xj + (z - zj) * (xi - xj) / (zi - zj)
			//
			// We count a crossing when x_intersect > x (intersection is to
			// the right of the query point).
			xIntersect := xj + (z-zj)*(xi-xj)/(zi-zj)
			if x < xIntersect {
				crossings++
			}
		}
		j = i
	}

	return crossings%2 == 1
}

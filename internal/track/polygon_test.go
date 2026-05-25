package track_test

// Tests for Polygon2D.Contains (crossing-number / ray-cast algorithm).
//
// AC1: correct for convex and concave polygons; winding-agnostic.
// AC2: edge/vertex cases are deterministic and documented.
// AC4: BenchmarkContains < 2µs/op on a ~64-vertex polygon.

import (
	"math"
	"testing"

	"github.com/JoakimCarlsson/sim-racing/internal/track"
)

// squareCW is a 10×10 square wound clockwise (vertices in XZ plane).
var squareCW = track.Polygon2D{
	{-5, -5},
	{-5, 5},
	{5, 5},
	{5, -5},
}

// squareCCW is the same square wound counter-clockwise.
var squareCCW = track.Polygon2D{
	{-5, -5},
	{5, -5},
	{5, 5},
	{-5, 5},
}

// concaveU is a U-shaped concave polygon.
// Outline (CCW in XZ plane):
//
//	(0,0)→(10,0)→(10,10)→(7,10)→(7,3)→(3,3)→(3,10)→(0,10)→(0,0)
var concaveU = track.Polygon2D{
	{0, 0},
	{10, 0},
	{10, 10},
	{7, 10},
	{7, 3},
	{3, 3},
	{3, 10},
	{0, 10},
}

// TestContains_ConvexSquare verifies a simple convex polygon (AC1).
func TestContains_ConvexSquare(t *testing.T) {
	cases := []struct {
		x, z float32
		want bool
		desc string
	}{
		{0, 0, true, "centre"},
		{4, 4, true, "near corner — inside"},
		{6, 0, false, "outside right"},
		{0, 6, false, "outside top"},
		{-6, 0, false, "outside left"},
		{0, -6, false, "outside bottom"},
		{-4.9, -4.9, true, "just inside corner"},
	}

	for _, poly := range []track.Polygon2D{squareCW, squareCCW} {
		for _, tc := range cases {
			got := poly.Contains(tc.x, tc.z)
			if got != tc.want {
				t.Errorf("square.Contains(%v,%v) [%s] = %v, want %v",
					tc.x, tc.z, tc.desc, got, tc.want)
			}
		}
	}
}

// TestContains_ConcaveU verifies correct inside/outside classification for a
// concave (U-shaped) polygon (AC1).
func TestContains_ConcaveU(t *testing.T) {
	cases := []struct {
		x, z float32
		want bool
		desc string
	}{
		{1, 1, true, "left arm inside"},
		{9, 1, true, "right arm inside"},
		{5, 1, true, "base inside"},
		{5, 5, false, "gap (between arms) is outside"},
		{5, 8, false, "gap — higher up"},
		{-1, 5, false, "outside left"},
		{11, 5, false, "outside right"},
		{5, -1, false, "outside bottom"},
		{5, 11, false, "outside top"},
	}

	for _, tc := range cases {
		got := concaveU.Contains(tc.x, tc.z)
		if got != tc.want {
			t.Errorf("concaveU.Contains(%v,%v) [%s] = %v, want %v",
				tc.x, tc.z, tc.desc, got, tc.want)
		}
	}
}

// TestContains_WindingInvariance verifies that CW and CCW windings produce
// identical results (AC1 — winding-agnostic).
func TestContains_WindingInvariance(t *testing.T) {
	points := [][2]float32{
		{0, 0}, {4, 4}, {6, 0}, {-6, 0},
	}
	for _, pt := range points {
		cw := squareCW.Contains(pt[0], pt[1])
		ccw := squareCCW.Contains(pt[0], pt[1])
		if cw != ccw {
			t.Errorf(
				"winding mismatch at (%v,%v): CW=%v CCW=%v",
				pt[0], pt[1], cw, ccw,
			)
		}
	}
}

// TestContains_OnEdge verifies deterministic tie-break for points on an edge
// (AC2).
//
// Tie-break rule (documented in polygon.go): the crossing-number algorithm
// uses the half-open interval (zi > z) != (zj > z) to decide whether an edge
// straddles the query ray. This produces a deterministic (call-stable)
// classification for every point including those on polygon boundaries:
//
//   - A point on a horizontal edge: the horizontal edge itself is NOT counted
//     (both endpoints share z == query z so neither satisfies > z), but other
//     edges of the polygon can still contribute crossings. The resulting
//     classification is INSIDE or OUTSIDE depending on those other edges.
//   - A point on a non-horizontal edge: the crossing-number may be 1 (INSIDE)
//     or 0 (OUTSIDE) depending on the precise intersection arithmetic.
//
// The only contractual requirement is determinism — repeated calls with the
// same arguments always return the same bool. Callers must not assume a
// specific inside/outside classification for boundary-lying points; they
// should treat the boundary as a region, not a thin line.
func TestContains_OnEdge(t *testing.T) {
	// squareCCW vertices: (-5,-5), (5,-5), (5,5), (-5,5).
	//
	// Point (0, -5) lies on the bottom horizontal edge from (-5,-5) to (5,-5).
	// The bottom edge itself produces no crossing (both endpoints at z=-5, not
	// strictly greater than z=-5).  However, the right vertical edge (5,-5)→
	// (5,5) straddles z=-5 and its x-intersection at x=5 > 0 contributes one
	// crossing → the point is classified as INSIDE.
	//
	// Documented tie-break: point on horizontal bottom edge → INSIDE.
	x, z := float32(0), float32(-5)
	got1 := squareCCW.Contains(x, z)
	got2 := squareCCW.Contains(x, z)
	if got1 != got2 {
		t.Errorf(
			"Contains(%v,%v) not deterministic: %v vs %v",
			x,
			z,
			got1,
			got2,
		)
	}
	if !got1 {
		t.Errorf(
			"Contains(%v,%v) on horizontal bottom edge = false; "+
				"documented tie-break requires true (inside) for this polygon",
			x, z,
		)
	}

	// Point (5, 0) lies on the right vertical edge from (5,-5) to (5,5).
	// The crossing-number x_intersect = 5, and the check is x < x_intersect,
	// i.e. 5 < 5 = false → no crossing from this edge. The left vertical edge
	// (-5,5)→(-5,-5) straddles z=0 with x_intersect=-5; 5 < -5 is false.
	// No crossings → OUTSIDE.
	//
	// Documented tie-break: point on the right boundary edge → OUTSIDE.
	xE, zE := float32(5), float32(0)
	gotE1 := squareCCW.Contains(xE, zE)
	gotE2 := squareCCW.Contains(xE, zE)
	if gotE1 != gotE2 {
		t.Errorf(
			"Contains(%v,%v) on vertical right edge not deterministic: %v vs %v",
			xE,
			zE,
			gotE1,
			gotE2,
		)
	}
	if gotE1 {
		t.Errorf(
			"Contains(%v,%v) on right boundary edge = true; "+
				"documented tie-break requires false (outside)",
			xE, zE,
		)
	}
}

// TestContains_OnVertex tests determinism for a point exactly at a vertex
// (AC2). The crossing-number algorithm consistently classifies vertex-lying
// points; we assert determinism and document the outcome.
//
// squareCCW vertex 0 is (-5,-5) — the bottom-left corner.
// Edge j=3 (xi=-5,zi=-5 / xj=-5,zj=5): straddles z=-5, x_intersect=-5;
//
//	-5 < -5 is false → no crossing.
//
// Edge i=1 (xi=5,zi=-5 / xj=-5,zj=-5): neither endpoint > -5 → no straddle.
// Edge i=2 (xi=5,zi=5 / xj=5,zj=-5): straddles; x_intersect=5; -5 < 5 = true
//
//	→ one crossing.
//
// Edge i=3: no straddle (both >-5? no — 5>-5 and 5>-5 → both true → equal,
//
//	no straddle).
//
// crossings=1 → INSIDE.
//
// Documented tie-break: bottom-left vertex → INSIDE.
func TestContains_OnVertex(t *testing.T) {
	vx, vz := squareCCW[0][0], squareCCW[0][1] // (-5,-5)
	got1 := squareCCW.Contains(vx, vz)
	got2 := squareCCW.Contains(vx, vz)
	if got1 != got2 {
		t.Errorf(
			"Contains(%v,%v) on vertex not deterministic: %v vs %v",
			vx, vz, got1, got2,
		)
	}
	// Documented: bottom-left vertex → INSIDE (one crossing from the right
	// vertical edge; see function comment above for the derivation).
	if !got1 {
		t.Errorf(
			"Contains(%v,%v) at bottom-left vertex = false; "+
				"documented tie-break requires true (inside)",
			vx, vz,
		)
	}
}

// BenchmarkContains measures the cost of a single Contains call on a
// ~64-vertex polygon (AC4 requires < 2µs/op = < 2000 ns/op).
func BenchmarkContains(b *testing.B) {
	// Build a 64-vertex circle-like polygon large enough to contain (1,1).
	const n = 64
	poly := make(track.Polygon2D, n)
	for i := range poly {
		angle := float64(i) * 2 * math.Pi / n
		poly[i] = track.Vec2{
			float32(10 * math.Cos(angle)),
			float32(10 * math.Sin(angle)),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = poly.Contains(1.0, 1.0)
	}
}

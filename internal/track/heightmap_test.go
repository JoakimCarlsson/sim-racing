package track_test

import (
	"bytes"
	"math"
	"testing"

	"github.com/JoakimCarlsson/sim-racing/internal/track"
)

// makeFlat3x3 builds a 3×3 heightmap with distinct heights at each grid point.
// Heights (row-major, z outer):
//
//	(0,0)=1  (1,0)=2  (2,0)=3
//	(0,1)=4  (1,1)=5  (2,1)=6
//	(0,2)=7  (1,2)=8  (2,2)=9
//
// OriginX=-1, OriginZ=-1, CellSize=1 → grid covers x∈[-1,1], z∈[-1,1].
func makeFlat3x3() *track.Heightmap {
	w, d := 3, 3
	heights := []float32{
		1, 2, 3,
		4, 5, 6,
		7, 8, 9,
	}
	normals := make([]float32, 3*w*d)
	for i := 0; i < w*d; i++ {
		normals[3*i+1] = 1 // up = [0,1,0]
	}
	return &track.Heightmap{
		OriginX:  -1,
		OriginZ:  -1,
		CellSize: 1,
		Width:    w,
		Depth:    d,
		Heights:  heights,
		Normals:  normals,
	}
}

// TestHeightmap_Sample_AtGridPoints verifies exact corner/grid-point reads.
func TestHeightmap_Sample_AtGridPoints(t *testing.T) {
	hm := makeFlat3x3()

	cases := []struct {
		x, z   float32
		wantH  float32
		wantNY float32
	}{
		{-1, -1, 1, 1}, // col=0,row=0
		{0, -1, 2, 1},  // col=1,row=0
		{1, -1, 3, 1},  // col=2,row=0
		{-1, 0, 4, 1},  // col=0,row=1
		{0, 0, 5, 1},   // col=1,row=1
		{1, 0, 6, 1},   // col=2,row=1
		{-1, 1, 7, 1},  // col=0,row=2
		{0, 1, 8, 1},   // col=1,row=2
		{1, 1, 9, 1},   // col=2,row=2
	}

	for _, tc := range cases {
		h, n, ok := hm.Sample(tc.x, tc.z)
		if !ok {
			t.Errorf("Sample(%v,%v): want ok=true, got false", tc.x, tc.z)
			continue
		}
		if math.Abs(float64(h-tc.wantH)) > 1e-5 {
			t.Errorf(
				"Sample(%v,%v): height want %v, got %v",
				tc.x,
				tc.z,
				tc.wantH,
				h,
			)
		}
		if math.Abs(float64(n[1]-tc.wantNY)) > 1e-5 {
			t.Errorf(
				"Sample(%v,%v): normal.Y want %v, got %v",
				tc.x,
				tc.z,
				tc.wantNY,
				n[1],
			)
		}
	}
}

// TestHeightmap_Sample_BilinearInterior checks a known interior point.
// At the centre of cell (col=0, row=0): x=-0.5, z=-0.5
// corners: h[0,0]=1, h[1,0]=2, h[0,1]=4, h[1,1]=5
// fx=0.5, fz=0.5 → bilinear = lerp(lerp(1,2,0.5), lerp(4,5,0.5), 0.5)
//
//	= lerp(1.5, 4.5, 0.5) = 3.0
func TestHeightmap_Sample_BilinearInterior(t *testing.T) {
	hm := makeFlat3x3()

	h, _, ok := hm.Sample(-0.5, -0.5)
	if !ok {
		t.Fatal("Sample(-0.5,-0.5): want ok=true, got false")
	}
	const want float32 = 3.0
	if math.Abs(float64(h-want)) > 1e-5 {
		t.Errorf("bilinear centre: want %v, got %v", want, h)
	}
}

// TestHeightmap_Sample_OutOfBounds verifies ok=false for points outside grid.
func TestHeightmap_Sample_OutOfBounds(t *testing.T) {
	hm := makeFlat3x3()

	outOfBounds := [][2]float32{
		{-2, 0}, // x too small
		{2, 0},  // x too large (max x = originX + (W-1)*cell = 1)
		{0, -2}, // z too small
		{0, 2},  // z too large
		{-10, -10},
		{10, 10},
	}

	for _, pt := range outOfBounds {
		_, _, ok := hm.Sample(pt[0], pt[1])
		if ok {
			t.Errorf("Sample(%v,%v): want ok=false, got true", pt[0], pt[1])
		}
	}
}

// TestHeightmap_Sample_EdgeAndCornerCells verifies samples right at the edges
// are still valid (on-boundary is in-bounds).
func TestHeightmap_Sample_EdgeAndCornerCells(t *testing.T) {
	hm := makeFlat3x3()

	// Edge samples exactly on the boundary should return ok=true.
	edgeSamples := [][2]float32{
		{-1, -1}, // top-left corner
		{1, -1},  // top-right corner
		{-1, 1},  // bottom-left corner
		{1, 1},   // bottom-right corner
		{0, -1},  // top edge midpoint
		{0, 1},   // bottom edge midpoint
		{-1, 0},  // left edge midpoint
		{1, 0},   // right edge midpoint
	}

	for _, pt := range edgeSamples {
		_, _, ok := hm.Sample(pt[0], pt[1])
		if !ok {
			t.Errorf(
				"Sample(%v,%v) on boundary: want ok=true, got false",
				pt[0],
				pt[1],
			)
		}
	}
}

// TestHeightmap_RoundTrip_Encode_Decode writes a heightmap to a buffer and
// reads it back, then asserts all fields are identical.
func TestHeightmap_RoundTrip_Encode_Decode(t *testing.T) {
	original := makeFlat3x3()

	var buf bytes.Buffer
	if err := track.WriteHeightmap(&buf, original); err != nil {
		t.Fatalf("WriteHeightmap: %v", err)
	}

	got, err := track.LoadHeightmap(&buf)
	if err != nil {
		t.Fatalf("LoadHeightmap: %v", err)
	}

	if got.OriginX != original.OriginX {
		t.Errorf("OriginX: want %v, got %v", original.OriginX, got.OriginX)
	}
	if got.OriginZ != original.OriginZ {
		t.Errorf("OriginZ: want %v, got %v", original.OriginZ, got.OriginZ)
	}
	if got.CellSize != original.CellSize {
		t.Errorf("CellSize: want %v, got %v", original.CellSize, got.CellSize)
	}
	if got.Width != original.Width {
		t.Errorf("Width: want %v, got %v", original.Width, got.Width)
	}
	if got.Depth != original.Depth {
		t.Errorf("Depth: want %v, got %v", original.Depth, got.Depth)
	}
	if len(got.Heights) != len(original.Heights) {
		t.Fatalf(
			"Heights len: want %v, got %v",
			len(original.Heights),
			len(got.Heights),
		)
	}
	for i := range original.Heights {
		if got.Heights[i] != original.Heights[i] {
			t.Errorf(
				"Heights[%d]: want %v, got %v",
				i,
				original.Heights[i],
				got.Heights[i],
			)
		}
	}
	if len(got.Normals) != len(original.Normals) {
		t.Fatalf(
			"Normals len: want %v, got %v",
			len(original.Normals),
			len(got.Normals),
		)
	}
	for i := range original.Normals {
		if got.Normals[i] != original.Normals[i] {
			t.Errorf(
				"Normals[%d]: want %v, got %v",
				i,
				original.Normals[i],
				got.Normals[i],
			)
		}
	}
}

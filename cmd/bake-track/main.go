// Command bake-track generates a track.height.bin heightmap file from a glTF
// track asset. For a flat track (circuit01) it reads the POSITION accessor's
// min/max AABB from the glTF JSON and emits a constant-height grid.
//
// Usage:
//
//	bake-track --in track.gltf --out track.height.bin [--cell-size 0.5] [--pad 5.0]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"

	"github.com/JoakimCarlsson/sim-racing/internal/track"
)

func main() {
	inFlag := flag.String("in", "", "path to input track.gltf (required)")
	outFlag := flag.String(
		"out",
		"",
		"path for output track.height.bin (required)",
	)
	cellSizeFlag := flag.Float64("cell-size", 0.5, "grid cell size in metres")
	padFlag := flag.Float64(
		"pad",
		5.0,
		"padding added to each side of AABB in metres",
	)
	flag.Parse()

	if *inFlag == "" || *outFlag == "" {
		flag.Usage()
		os.Exit(1)
	}
	if *cellSizeFlag <= 0 {
		log.Fatalf(
			"bake-track: --cell-size must be positive, got %v",
			*cellSizeFlag,
		)
	}
	if *padFlag < 0 {
		log.Fatalf("bake-track: --pad must be non-negative, got %v", *padFlag)
	}

	aabb, meshY, err := readGLTFPositionAABB(*inFlag)
	if err != nil {
		log.Fatalf("bake-track: read glTF: %v", err)
	}

	cellSize := float32(*cellSizeFlag)
	pad := float32(*padFlag)

	originX := aabb.minX - pad
	originZ := aabb.minZ - pad
	maxX := aabb.maxX + pad
	maxZ := aabb.maxZ + pad

	// Number of cells = ceil((max-origin)/cellSize), clamped to at least 2.
	width := int(math.Ceil(float64((maxX-originX)/cellSize))) + 1
	depth := int(math.Ceil(float64((maxZ-originZ)/cellSize))) + 1
	if width < 2 {
		width = 2
	}
	if depth < 2 {
		depth = 2
	}

	n := width * depth
	heights := make([]float32, n)
	normals := make([]float32, 3*n)
	for i := 0; i < n; i++ {
		heights[i] = meshY
		// Normal = [0, 1, 0] (flat, Y-up)
		normals[3*i+0] = 0
		normals[3*i+1] = 1
		normals[3*i+2] = 0
	}

	hm := &track.Heightmap{
		OriginX:  originX,
		OriginZ:  originZ,
		CellSize: cellSize,
		Width:    width,
		Depth:    depth,
		Heights:  heights,
		Normals:  normals,
	}

	f, err := os.Create(*outFlag)
	if err != nil {
		log.Fatalf("bake-track: create output: %v", err)
	}
	defer func() { _ = f.Close() }()

	if err := track.WriteHeightmap(f, hm); err != nil {
		log.Fatalf("bake-track: write heightmap: %v", err)
	}

	info, err := f.Stat()
	if err != nil {
		log.Fatalf("bake-track: stat output: %v", err)
	}

	fmt.Printf(
		"bake-track: wrote %dx%d grid (cellSize=%.4gm, AABB [%.4g,%.4g] x [%.4g,%.4g]) -> %s (%d bytes)\n",
		width,
		depth,
		cellSize,
		originX,
		maxX,
		originZ,
		maxZ,
		*outFlag,
		info.Size(),
	)
}

// aabbXZ holds the 2D AABB extracted from a glTF POSITION accessor.
type aabbXZ struct {
	minX, maxX float32
	minZ, maxZ float32
}

// gltfAccessor is the minimal subset of a glTF 2.0 accessor we need.
type gltfAccessor struct {
	Min []float64 `json:"min"`
	Max []float64 `json:"max"`
	// ComponentType 5126 = FLOAT, Type "VEC3" — we rely on the caller to pick
	// the POSITION accessor which must be VEC3 FLOAT.
	ComponentType int    `json:"componentType"`
	Type          string `json:"type"`
}

type gltfPrimitive struct {
	Attributes map[string]int `json:"attributes"`
}

type gltfMesh struct {
	Primitives []gltfPrimitive `json:"primitives"`
}

type gltfRoot struct {
	Meshes    []gltfMesh     `json:"meshes"`
	Accessors []gltfAccessor `json:"accessors"`
}

// readGLTFPositionAABB opens inPath, parses the glTF JSON, finds the first
// POSITION accessor and returns its XZ AABB and the Y value (meshY).
func readGLTFPositionAABB(inPath string) (aabbXZ, float32, error) {
	f, err := os.Open(inPath)
	if err != nil {
		return aabbXZ{}, 0, fmt.Errorf("open %s: %w", inPath, err)
	}
	defer func() { _ = f.Close() }()

	var root gltfRoot
	dec := json.NewDecoder(f)
	if err := dec.Decode(&root); err != nil {
		return aabbXZ{}, 0, fmt.Errorf("JSON decode: %w", err)
	}

	if len(root.Meshes) == 0 {
		return aabbXZ{}, 0, fmt.Errorf("no meshes found in glTF")
	}
	if len(root.Meshes[0].Primitives) == 0 {
		return aabbXZ{}, 0, fmt.Errorf("no primitives in first mesh")
	}

	posIdx, ok := root.Meshes[0].Primitives[0].Attributes["POSITION"]
	if !ok {
		return aabbXZ{}, 0, fmt.Errorf("POSITION attribute not found")
	}

	if posIdx < 0 || posIdx >= len(root.Accessors) {
		return aabbXZ{}, 0,
			fmt.Errorf("POSITION accessor index %d out of range", posIdx)
	}

	acc := root.Accessors[posIdx]
	if len(acc.Min) < 3 || len(acc.Max) < 3 {
		return aabbXZ{}, 0,
			fmt.Errorf(
				"POSITION accessor min/max must have 3 components, got min=%d max=%d",
				len(acc.Min),
				len(acc.Max),
			)
	}

	aabb := aabbXZ{
		minX: float32(acc.Min[0]),
		maxX: float32(acc.Max[0]),
		minZ: float32(acc.Min[2]),
		maxZ: float32(acc.Max[2]),
	}
	meshY := float32(acc.Min[1]) // flat mesh: min Y == max Y

	return aabb, meshY, nil
}

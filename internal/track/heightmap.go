package track

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// MagicBytes is the 4-byte magic header for the .height.bin format.
var MagicBytes = [4]byte{'S', 'R', 'H', 'M'}

// FormatVersion is the binary format version for .height.bin files.
const FormatVersion uint16 = 1

// Heightmap is a regular-grid elevation map defined on the XZ plane.
// Heights and Normals are stored row-major (z outer, x inner):
// index = row*Width + col, where col = x-dimension, row = z-dimension.
//
// Normals are stored as packed XYZ triples: Normals[3*i], Normals[3*i+1],
// Normals[3*i+2] for the i-th cell.
type Heightmap struct {
	OriginX  float32
	OriginZ  float32
	CellSize float32
	Width    int
	Depth    int
	Heights  []float32
	Normals  []float32
}

// Sample returns the bilinearly interpolated height and normal at world-space
// position (x, z). Returns ok=false if (x, z) is outside the grid boundary.
//
// Grid boundary: x in [OriginX, OriginX+(Width-1)*CellSize],
// z in [OriginZ, OriginZ+(Depth-1)*CellSize].
func (h *Heightmap) Sample(
	x, z float32,
) (height float32, normal [3]float32, ok bool) {
	maxX := h.OriginX + float32(h.Width-1)*h.CellSize
	maxZ := h.OriginZ + float32(h.Depth-1)*h.CellSize

	if x < h.OriginX || x > maxX || z < h.OriginZ || z > maxZ {
		return 0, [3]float32{}, false
	}

	// Compute fractional column and row.
	fc := (x - h.OriginX) / h.CellSize
	fr := (z - h.OriginZ) / h.CellSize

	col := int(fc)
	row := int(fr)

	// Clamp to prevent out-of-bounds on exact max boundary.
	if col >= h.Width-1 {
		col = h.Width - 2
	}
	if row >= h.Depth-1 {
		row = h.Depth - 2
	}

	// Fractional offsets within the cell [0,1].
	fx := fc - float32(col)
	fz := fr - float32(row)

	// Indices of the four corners.
	i00 := row*h.Width + col
	i10 := row*h.Width + (col + 1)
	i01 := (row+1)*h.Width + col
	i11 := (row+1)*h.Width + (col + 1)

	// Bilinear interpolation of height.
	h00 := h.Heights[i00]
	h10 := h.Heights[i10]
	h01 := h.Heights[i01]
	h11 := h.Heights[i11]

	height = lerp(lerp(h00, h10, fx), lerp(h01, h11, fx), fz)

	// Bilinear interpolation of normals.
	for k := 0; k < 3; k++ {
		n00 := h.Normals[3*i00+k]
		n10 := h.Normals[3*i10+k]
		n01 := h.Normals[3*i01+k]
		n11 := h.Normals[3*i11+k]
		normal[k] = lerp(lerp(n00, n10, fx), lerp(n01, n11, fx), fz)
	}

	return height, normal, true
}

// lerp linearly interpolates between a and b by t.
func lerp(a, b, t float32) float32 {
	return a + t*(b-a)
}

// LoadHeightmap reads a Heightmap from r in the .height.bin binary format.
// Binary layout (all little-endian):
//
//	magic    [4]byte  = "SRHM"
//	version  uint16   = 1
//	reserved uint16   = 0
//	originX  float32
//	originZ  float32
//	cellSize float32
//	width    uint32
//	depth    uint32
//	heights  [width*depth]float32
//	normals  [3*width*depth]float32
func LoadHeightmap(r io.Reader) (*Heightmap, error) {
	var magic [4]byte
	if _, err := io.ReadFull(r, magic[:]); err != nil {
		return nil, fmt.Errorf("heightmap: read magic: %w", err)
	}
	if magic != MagicBytes {
		return nil,
			fmt.Errorf(
				"heightmap: invalid magic %q, want %q",
				magic,
				MagicBytes,
			)
	}

	var version uint16
	if err := binary.Read(r, binary.LittleEndian, &version); err != nil {
		return nil, fmt.Errorf("heightmap: read version: %w", err)
	}
	if version != FormatVersion {
		return nil,
			fmt.Errorf(
				"heightmap: unsupported version %d, want %d",
				version,
				FormatVersion,
			)
	}

	var reserved uint16
	if err := binary.Read(r, binary.LittleEndian, &reserved); err != nil {
		return nil, fmt.Errorf("heightmap: read reserved: %w", err)
	}

	var originX, originZ, cellSize float32
	if err := binary.Read(r, binary.LittleEndian, &originX); err != nil {
		return nil, fmt.Errorf("heightmap: read originX: %w", err)
	}
	if err := binary.Read(r, binary.LittleEndian, &originZ); err != nil {
		return nil, fmt.Errorf("heightmap: read originZ: %w", err)
	}
	if err := binary.Read(r, binary.LittleEndian, &cellSize); err != nil {
		return nil, fmt.Errorf("heightmap: read cellSize: %w", err)
	}

	if cellSize <= 0 {
		return nil,
			fmt.Errorf("heightmap: cellSize must be positive, got %v", cellSize)
	}

	var width, depth uint32
	if err := binary.Read(r, binary.LittleEndian, &width); err != nil {
		return nil, fmt.Errorf("heightmap: read width: %w", err)
	}
	if err := binary.Read(r, binary.LittleEndian, &depth); err != nil {
		return nil, fmt.Errorf("heightmap: read depth: %w", err)
	}

	if width == 0 || depth == 0 {
		return nil,
			fmt.Errorf(
				"heightmap: dimensions must be positive, got %dx%d",
				width,
				depth,
			)
	}

	n := int(width) * int(depth)

	heights := make([]float32, n)
	if err := binary.Read(r, binary.LittleEndian, heights); err != nil {
		return nil, fmt.Errorf("heightmap: read heights: %w", err)
	}

	normals := make([]float32, 3*n)
	if err := binary.Read(r, binary.LittleEndian, normals); err != nil {
		return nil, fmt.Errorf("heightmap: read normals: %w", err)
	}

	return &Heightmap{
		OriginX:  originX,
		OriginZ:  originZ,
		CellSize: cellSize,
		Width:    int(width),
		Depth:    int(depth),
		Heights:  heights,
		Normals:  normals,
	}, nil
}

// LoadHeightmapFile is a convenience wrapper around LoadHeightmap that opens
// the named file.
func LoadHeightmapFile(path string) (*Heightmap, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("heightmap: open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	return LoadHeightmap(f)
}

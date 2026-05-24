package track

import (
	"encoding/binary"
	"fmt"
	"io"
)

// WriteHeightmap encodes hm to w in the .height.bin binary format.
// See LoadHeightmap for the exact binary layout.
func WriteHeightmap(w io.Writer, hm *Heightmap) error {
	if hm.CellSize <= 0 {
		return fmt.Errorf(
			"heightmap: CellSize must be positive, got %v",
			hm.CellSize,
		)
	}
	if hm.Width <= 0 || hm.Depth <= 0 {
		return fmt.Errorf(
			"heightmap: dimensions must be positive, got %dx%d",
			hm.Width,
			hm.Depth,
		)
	}

	n := hm.Width * hm.Depth
	if len(hm.Heights) != n {
		return fmt.Errorf(
			"heightmap: Heights length %d != Width*Depth %d",
			len(hm.Heights),
			n,
		)
	}
	if len(hm.Normals) != 3*n {
		return fmt.Errorf(
			"heightmap: Normals length %d != 3*Width*Depth %d",
			len(hm.Normals),
			3*n,
		)
	}

	if _, err := w.Write(MagicBytes[:]); err != nil {
		return fmt.Errorf("heightmap: write magic: %w", err)
	}

	if err := binary.Write(w, binary.LittleEndian, FormatVersion); err != nil {
		return fmt.Errorf("heightmap: write version: %w", err)
	}

	var reserved uint16
	if err := binary.Write(w, binary.LittleEndian, reserved); err != nil {
		return fmt.Errorf("heightmap: write reserved: %w", err)
	}

	if err := binary.Write(w, binary.LittleEndian, hm.OriginX); err != nil {
		return fmt.Errorf("heightmap: write originX: %w", err)
	}
	if err := binary.Write(w, binary.LittleEndian, hm.OriginZ); err != nil {
		return fmt.Errorf("heightmap: write originZ: %w", err)
	}
	if err := binary.Write(w, binary.LittleEndian, hm.CellSize); err != nil {
		return fmt.Errorf("heightmap: write cellSize: %w", err)
	}

	if err := binary.Write(w, binary.LittleEndian, uint32(hm.Width)); err != nil {
		return fmt.Errorf("heightmap: write width: %w", err)
	}
	if err := binary.Write(w, binary.LittleEndian, uint32(hm.Depth)); err != nil {
		return fmt.Errorf("heightmap: write depth: %w", err)
	}

	if err := binary.Write(w, binary.LittleEndian, hm.Heights); err != nil {
		return fmt.Errorf("heightmap: write heights: %w", err)
	}

	if err := binary.Write(w, binary.LittleEndian, hm.Normals); err != nil {
		return fmt.Errorf("heightmap: write normals: %w", err)
	}

	return nil
}

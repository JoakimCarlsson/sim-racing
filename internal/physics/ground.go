package physics

// GroundSampler is the pure interface that provides terrain height and surface
// normal at a given world-space (x, z) coordinate.
//
// Sample returns:
//   - height: the terrain elevation at (x, z) in metres (world Y).
//   - normal: the outward surface unit normal [x, y, z] at (x, z).
//     For flat ground this is [0, 1, 0].
//   - ok: false when (x, z) is outside the sampler's valid domain; the
//     caller should treat the wheel as airborne (no ground contact).
//
// Implementations must be pure (no I/O, no goroutines) so that physics.Step
// remains deterministic and WASM-safe.
type GroundSampler interface {
	Sample(x, z float32) (height float32, normal [3]float32, ok bool)
}

// flatGround is an infinite flat plane at a fixed elevation.
type flatGround struct {
	height float32
}

// FlatGround returns a GroundSampler that reports a flat horizontal surface at
// the given height (metres, world Y). ok is always true — the plane is infinite.
// This is the default sampler used when no heightmap is available.
func FlatGround(height float32) GroundSampler {
	return flatGround{height: height}
}

// Sample implements GroundSampler.
// The normal of a flat ground plane pointing up is always [0, 1, 0].
func (f flatGround) Sample(_, _ float32) (float32, [3]float32, bool) {
	return f.height, [3]float32{0, 1, 0}, true
}

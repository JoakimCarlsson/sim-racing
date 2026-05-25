package sim_test

// Compile-time assertion: *track.Heightmap satisfies physics.GroundSampler.
// This ensures the interface contract is upheld without importing track into
// the physics package (preserving physics' stdlib-only requirement).

import (
	"github.com/JoakimCarlsson/sim-racing/internal/physics"
	"github.com/JoakimCarlsson/sim-racing/internal/track"
)

var _ physics.GroundSampler = (*track.Heightmap)(nil)

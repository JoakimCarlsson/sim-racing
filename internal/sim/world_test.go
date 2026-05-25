package sim

// Tests for PlayerSim.WheelsOff — the per-tick wheels-outside-limits counter.
//
// AC3: WheelsOff (uint8, 0..4) is updated each tick; 0 when world.Limits is
//      nil/empty.

import (
	"testing"

	"github.com/JoakimCarlsson/sim-racing/internal/physics"
	"github.com/JoakimCarlsson/sim-racing/internal/track"
)

// nullSink silently discards all snapshots (distinct name from other test sinks).
type nullSink struct{}

func (nullSink) SendSnapshot(_ []byte, _ uint32) {}

// worldWithLimits creates a minimal World with given limits polygon, adds one
// player at pos with identity orientation, and returns both.
func worldWithLimits(
	limits track.Polygon2D,
	pos [3]float32,
) (*World, *PlayerSim) {
	w := New()
	w.Limits = limits

	p := &PlayerSim{
		ID:     1,
		Source: &emptySource{},
		Sink:   nullSink{},
		State: physics.State{
			Position:    pos,
			Orientation: [4]float32{0, 0, 0, 1},
		},
		Constants: physics.DefaultConstants,
	}
	w.AddPlayer(p)
	return w, p
}

// TestWheelsOff_AllInside verifies WheelsOff=0 when the car is well inside
// the limits polygon (AC3).
func TestWheelsOff_AllInside(t *testing.T) {
	// Large square that encloses any car sitting at the origin.
	limits := track.Polygon2D{
		{-100, -100},
		{100, -100},
		{100, 100},
		{-100, 100},
	}
	w, p := worldWithLimits(limits, [3]float32{0, 1, 0})

	w.tick(1.0 / 60.0)

	if p.WheelsOff != 0 {
		t.Errorf("WheelsOff = %d, want 0 (all inside)", p.WheelsOff)
	}
}

// TestWheelsOff_AllOutside verifies WheelsOff=4 when the car is entirely
// outside the limits polygon (AC3).
func TestWheelsOff_AllOutside(t *testing.T) {
	// Small square far from origin.
	limits := track.Polygon2D{
		{-1, -1},
		{1, -1},
		{1, 1},
		{-1, 1},
	}
	// Car at (100, 1, 100) — all wheels far outside the tiny polygon.
	w, p := worldWithLimits(limits, [3]float32{100, 1, 100})

	w.tick(1.0 / 60.0)

	if p.WheelsOff != 4 {
		t.Errorf("WheelsOff = %d, want 4 (all outside)", p.WheelsOff)
	}
}

// TestWheelsOff_TwoOutside verifies partial wheel counts (AC3).
// Polygon covers X in [0, 100], Z in [-100, 100].
// CoM is placed so FL+RL are inside (positive X) and FR+RR are outside
// (negative X).
func TestWheelsOff_TwoOutside(t *testing.T) {
	limits := track.Polygon2D{
		{0, -100},
		{100, -100},
		{100, 100},
		{0, 100},
	}
	c := physics.DefaultConstants
	halfTrack := c.TrackWidth / 2 // ~0.775 m

	// WheelXZ body-frame offsets (identity yaw):
	//   FL: pos.X + halfTrack   (right side)
	//   FR: pos.X - halfTrack   (left side)
	//   RL: pos.X + halfTrack
	//   RR: pos.X - halfTrack
	// Place CoM at X = halfTrack/2:
	//   FL,RL: X = 3*halfTrack/2 > 0  → inside
	//   FR,RR: X = -halfTrack/2 < 0   → outside  → WheelsOff = 2
	comX := halfTrack / 2
	w, p := worldWithLimits(limits, [3]float32{comX, 1, 0})

	w.tick(1.0 / 60.0)

	if p.WheelsOff != 2 {
		t.Errorf("WheelsOff = %d, want 2 (FR+RR outside)", p.WheelsOff)
	}
}

// TestWheelsOff_NoLimitsPolygon verifies WheelsOff=0 when world.Limits is
// nil/empty (AC3).
func TestWheelsOff_NoLimitsPolygon(t *testing.T) {
	w := New()
	// Intentionally leave w.Limits at zero value (nil polygon).

	p := &PlayerSim{
		ID:     1,
		Source: &emptySource{},
		Sink:   nullSink{},
		State: physics.State{
			Position:    [3]float32{0, 1, 0},
			Orientation: [4]float32{0, 0, 0, 1},
		},
		Constants: physics.DefaultConstants,
	}
	w.AddPlayer(p)

	w.tick(1.0 / 60.0)

	if p.WheelsOff != 0 {
		t.Errorf("WheelsOff = %d, want 0 (no limits polygon)", p.WheelsOff)
	}
}

package sim

import (
	"context"
	"log"
	"time"

	"github.com/JoakimCarlsson/sim-racing/internal/physics"
	"github.com/JoakimCarlsson/sim-racing/internal/protocol"
	"github.com/JoakimCarlsson/sim-racing/internal/track"
)

// maxCatchupTicks is the maximum number of queued inputs applied per player
// per tick. This caps any catch-up burst to avoid starvation of other
// processing.
const maxCatchupTicks = 4

// Run starts the 60 Hz authoritative tick loop. It blocks until ctx is
// cancelled. Run is intended to be launched in its own goroutine.
func (w *World) Run(ctx context.Context) {
	interval := time.Duration(float64(time.Second) / float64(w.TickHz))
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	dt := float32(1.0 / float64(w.TickHz))

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.tick(dt)
		}
	}
}

// tick is the unexported per-tick step used by Run and directly by tests for
// deterministic step-based simulation (AC4).
func (w *World) tick(dt float32) {
	w.mu.Lock()
	// Collect pointers to all players under the lock so we can release it
	// before touching physics (physics.Step is pure and needs no lock).
	ps := make([]*PlayerSim, 0, len(w.players))
	for _, p := range w.players {
		ps = append(ps, p)
	}
	ground := w.Ground
	limits := w.Limits
	w.mu.Unlock()

	// Fall back to flat ground if the World has no sampler configured.
	if ground == nil {
		ground = physics.FlatGround(0)
	}

	scratch := make([]protocol.ClientInput, w.MaxInputsPerTick)
	for _, p := range ps {
		// Capture position before physics advances it, for crossing detection.
		prevPos := track.Vec3(p.State.Position)

		n := p.Source.DrainInputs(scratch[:w.MaxInputsPerTick])

		// Apply inputs in sequence order, skipping any seq <= LastAppliedSeq.
		// Cap the total inputs applied this tick to maxCatchupTicks to prevent
		// burst catch-up from monopolising the tick budget.
		applied := 0
		for i := 0; i < n && applied < maxCatchupTicks; i++ {
			in := scratch[i]
			if in.Seq <= p.LastAppliedSeq {
				continue
			}
			p.State = physics.Step(p.State, in.Input, p.Constants, ground, dt)
			p.LastAppliedSeq = in.Seq
			applied++
		}

		// If no inputs arrived this tick, still advance the simulation with
		// a zero (coast) input so the car continues to respond to physics
		// (e.g. drag, rolling resistance).
		if applied == 0 {
			p.State = physics.Step(
				p.State,
				physics.Input{},
				p.Constants,
				ground,
				dt,
			)
		}

		// Count how many wheel contact patches are outside the limits polygon.
		p.WheelsOff = countWheelsOff(p.State, p.Constants, limits)

		// Detect sector / start-finish crossings using the segment
		// [prevPos, curPos].  Skip the very first tick (HasPrev==false)
		// to avoid a spurious crossing when PrevPos is the zero vector.
		curPos := track.Vec3(p.State.Position)
		if p.HasPrev {
			checkSectorCrossings(w, p, p.PrevPos, curPos)
		}
		p.PrevPos = prevPos
		p.HasPrev = true
	}
}

// countWheelsOff returns the number of wheel contact patches (0..4) that lie
// outside the given limits polygon. Returns 0 when limits is nil or has fewer
// than 3 vertices (check disabled).
func countWheelsOff(
	s physics.State,
	c physics.Constants,
	limits track.Polygon2D,
) uint8 {
	if len(limits) < 3 {
		return 0
	}
	pts := physics.WheelXZ(s, c)
	var off uint8
	for _, pt := range pts {
		if !limits.Contains(pt[0], pt[1]) {
			off++
		}
	}
	return off
}

// checkSectorCrossings detects whether the segment [prev, cur] crosses any
// sector gate or the start/finish line and logs each event. Sector crossings
// are only logged when they occur in order (sectors[0] → [1] → … → startFinish).
//
// This function is called from tick after physics.Step so the caller can supply
// the pre-step position as prev and the post-step position as cur.
//
// checkSectorCrossings is package-private so it can be called directly from
// sector_test.go without running the full physics tick loop.
func checkSectorCrossings(w *World, p *PlayerSim, prev, cur track.Vec3) {
	w.mu.Lock()
	sectors := w.Sectors
	startFinish := w.StartFinish
	w.mu.Unlock()

	// Check the next expected sector gate.
	if len(sectors) > 0 && p.NextSector < len(sectors) {
		gate := sectors[p.NextSector]
		if crossed, _ := gate.Crossed(prev, cur); crossed {
			sectorNum := p.NextSector + 1 // 1-based for readability
			log.Printf(
				"sector crossing playerID=%d sector %d",
				p.ID,
				sectorNum,
			)
			p.NextSector++
		}
		// Do not fall through to startFinish if no sectors remain yet.
		if p.NextSector < len(sectors) {
			return
		}
	}

	// Check the start/finish gate (only valid after all sectors are cleared,
	// or when no sectors are configured).
	if allSectorsCleared(p, sectors) {
		sfNonZero := startFinish.Normal[0] != 0 ||
			startFinish.Normal[1] != 0 ||
			startFinish.Normal[2] != 0
		if sfNonZero {
			if crossed, _ := startFinish.Crossed(prev, cur); crossed {
				log.Printf(
					"sector crossing playerID=%d startFinish",
					p.ID,
				)
				// Reset for the next lap.
				p.NextSector = 0
			}
		}
	}
}

// allSectorsCleared reports whether the player has traversed all configured
// sector gates and is now eligible to cross the start/finish line.
func allSectorsCleared(p *PlayerSim, sectors []track.Plane) bool {
	return len(sectors) == 0 || p.NextSector >= len(sectors)
}

package sim

import (
	"context"
	"time"

	"github.com/JoakimCarlsson/sim-racing/internal/physics"
	"github.com/JoakimCarlsson/sim-racing/internal/protocol"
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
	w.mu.Unlock()

	scratch := make([]protocol.ClientInput, w.MaxInputsPerTick)
	for _, p := range ps {
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
			p.State = physics.Step(p.State, in.Input, p.Constants, dt)
			p.LastAppliedSeq = in.Seq
			applied++
		}

		// If no inputs arrived this tick, still advance the simulation with
		// a zero (coast) input so the car continues to respond to physics
		// (e.g. drag, rolling resistance).
		if applied == 0 {
			p.State = physics.Step(p.State, physics.Input{}, p.Constants, dt)
		}
	}
}

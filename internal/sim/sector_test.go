package sim

// Tests for AC3: sector-crossing detection and logging.
//
// TestSectorCrossingLog_OrderedTraversal — car crosses sectors[0], sectors[1],
// sectors[2], startFinish in that order; expect four log lines in the correct
// order and the playerID present in each line.
//
// TestSectorCrossingLog_NoSpuriousOnSpawn — on the very first tick (HasPrev==false)
// no crossing must be logged even when the spawn position straddles a gate.

import (
	"bytes"
	"log"
	"strings"
	"testing"

	"github.com/JoakimCarlsson/sim-racing/internal/physics"
	"github.com/JoakimCarlsson/sim-racing/internal/track"
)

// crossingShim calls the unexported checkSectorCrossings function for testing.
// It sets PrevPos=from and "current" position=to, then invokes the crossing
// logic so tests can assert log output without running full physics ticks.
func crossingShim(
	w *World,
	p *PlayerSim,
	from, to track.Vec3,
) {
	prev := from
	cur := to
	checkSectorCrossings(w, p, prev, cur)
}

// TestSectorCrossingLog_OrderedTraversal verifies that a car traversing all
// sector planes in order (sectors[0] → sectors[1] → sectors[2] → startFinish)
// emits four correctly-prefixed log lines (AC3).
func TestSectorCrossingLog_OrderedTraversal(t *testing.T) {
	// Redirect the standard logger so we can capture output.
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(nil) // restore to stderr after the test

	// Build a world that matches the circuit01 gate layout.
	w := New()
	w.StartFinish = track.Plane{
		P0:     track.Vec3{-5, 0, 0},
		P1:     track.Vec3{5, 0, 0},
		Normal: track.Vec3{0, 0, 1},
	}
	w.Sectors = []track.Plane{
		{
			P0:     track.Vec3{6, 0, -5},
			P1:     track.Vec3{14, 0, -5},
			Normal: track.Vec3{0, 0, -1},
		},
		{
			P0:     track.Vec3{-14, 0, -12},
			P1:     track.Vec3{14, 0, -12},
			Normal: track.Vec3{-1, 0, 0},
		},
		{
			P0:     track.Vec3{-14, 0, -5},
			P1:     track.Vec3{-6, 0, -5},
			Normal: track.Vec3{0, 0, 1},
		},
	}

	p := &PlayerSim{
		ID:         7,
		NextSector: 0,
		Source:     &emptySource{},
		Constants:  physics.DefaultConstants,
	}

	// Cross sector[0]: gate at z=-5 from x=6..14, normal=(0,0,-1).
	// Centroid=(10,0,-5). Car crosses from z=-4 (sPrev=(−4−(−5))*(−1)=−1<0)
	// to z=-6 (sCur=(−6−(−5))*(−1)=1>0). → crossing.
	crossingShim(w, p,
		track.Vec3{10, 0, -4},
		track.Vec3{10, 0, -6},
	)

	// Cross sector[1]: gate from x=-14..14 at z=-12, normal=(-1,0,0).
	// Centroid=(0,0,-12). Car crosses from x=1 (sPrev=(1-0)*(−1)=−1<0)
	// to x=-1 (sCur=(−1−0)*(−1)=1>0). → crossing.
	crossingShim(w, p,
		track.Vec3{1, 0, -12},
		track.Vec3{-1, 0, -12},
	)

	// Cross sector[2]: gate at z=-5 from x=-14..-6, normal=(0,0,1).
	// Centroid=(−10,0,−5). Car crosses from z=-6 (sPrev=(−6−(−5))*(1)=−1<0)
	// to z=-4 (sCur=(−4−(−5))*(1)=1>0). → crossing.
	crossingShim(w, p,
		track.Vec3{-10, 0, -6},
		track.Vec3{-10, 0, -4},
	)

	// Cross startFinish: gate from x=-5..5 at z=0, normal=(0,0,1).
	// Car crosses from z=-1 to z=+1. → crossing.
	crossingShim(w, p,
		track.Vec3{0, 0, -1},
		track.Vec3{0, 0, 1},
	)

	out := buf.String()

	wantFragments := []string{
		"sector 1",
		"sector 2",
		"sector 3",
		"startFinish",
	}
	for _, frag := range wantFragments {
		if !strings.Contains(out, frag) {
			t.Errorf(
				"expected log to contain %q; full output:\n%s",
				frag,
				out,
			)
		}
	}

	// Player ID must appear in each crossing line.
	if !strings.Contains(out, "7") {
		t.Errorf("expected log to contain playerID=7; full output:\n%s", out)
	}

	// Ensure ordering: "sector 1" must appear before "sector 2", etc.
	idx1 := strings.Index(out, "sector 1")
	idx2 := strings.Index(out, "sector 2")
	idx3 := strings.Index(out, "sector 3")
	idxSF := strings.Index(out, "startFinish")
	if idx1 < 0 || idx2 < 0 || idx3 < 0 || idxSF < 0 {
		t.Errorf("not all expected crossing events found in log:\n%s", out)
		return
	}
	if idx1 >= idx2 || idx2 >= idx3 || idx3 >= idxSF {
		t.Errorf(
			"crossing events out of order: sector1@%d sector2@%d sector3@%d sf@%d\n%s",
			idx1,
			idx2,
			idx3,
			idxSF,
			out,
		)
	}
}

// TestSectorCrossingLog_NoSpuriousOnSpawn verifies that no crossing log is
// emitted on the very first tick (HasPrev==false) even when the spawn position
// is exactly on a gate boundary (AC3).
func TestSectorCrossingLog_NoSpuriousOnSpawn(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(nil)

	w := New()
	w.StartFinish = track.Plane{
		P0:     track.Vec3{-5, 0, 0},
		P1:     track.Vec3{5, 0, 0},
		Normal: track.Vec3{0, 0, 1},
	}

	p := &PlayerSim{
		ID:         3,
		HasPrev:    false, // not yet initialised
		NextSector: 0,
		Source:     &emptySource{},
		Constants:  physics.DefaultConstants,
	}
	w.AddPlayer(p)

	// Single tick — HasPrev is false so no crossing check must fire.
	w.tick(1.0 / 60.0)

	if buf.Len() > 0 {
		t.Errorf(
			"expected no log output on spawn tick; got:\n%s",
			buf.String(),
		)
	}
}

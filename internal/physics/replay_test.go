package physics_test

// TestReplayDeterminism locks in the determinism contract for the physics
// package by replaying a pre-recorded input fixture and comparing the resulting
// state-stream hash against a committed golden digest.
//
// Two sub-tests:
//   - "golden_hash"          — amd64 only; compares against committed golden.
//   - "structural_invariants"— every arch; checks finite values, unit
//     quaternion, non-negative WheelLoad, RPM in [IdleRPM, RedlineRPM].
//
// # Fixture format (20 bytes per record, little-endian)
//
//	[0:4]   Seq       uint32
//	[4:8]   Throttle  float32
//	[8:12]  Brake     float32
//	[12:16] Steer     float32
//	[16]    Gear      int8  (stored as byte)
//	[17]    Handbrake uint8
//	[18:20] _         padding
//
// # Hash routine
//
// SYNC NOTE: hashStateStream MUST remain byte-for-byte identical to
// replayAndHash in cmd/replaygen/main.go. If you change field order or
// serialisation here, update that file too (and vice versa). Field order:
// Position[0..2], Orientation[0..3], LinearVel[0..2], AngularVel[0..2], RPM,
// Gear, WheelLoad[0..3], WheelSlip[0..3], Grounded.

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/JoakimCarlsson/sim-racing/internal/physics"
)

const (
	fixtureFile    = "testdata/lap_inputs.bin"
	goldenFile     = "testdata/lap_inputs.golden"
	fixtureRecSize = 20 // bytes per record — see package doc comment above
	replayHz       = 60
	replayDT       = float32(1.0) / float32(replayHz)
)

// replayRecord mirrors the on-disk record layout.
type replayRecord struct {
	Seq       uint32
	Throttle  float32
	Brake     float32
	Steer     float32
	Gear      int8
	Handbrake uint8
}

// loadFixture reads all records from the binary fixture file.
func loadFixture(t *testing.T) []replayRecord {
	t.Helper()
	data, err := os.ReadFile(fixtureFile)
	if err != nil {
		t.Fatalf("loadFixture: %v", err)
	}
	if len(data)%fixtureRecSize != 0 {
		t.Fatalf(
			"fixture size %d is not a multiple of %d bytes",
			len(data), fixtureRecSize,
		)
	}
	n := len(data) / fixtureRecSize
	records := make([]replayRecord, n)
	for i := range records {
		off := i * fixtureRecSize
		records[i] = replayRecord{
			Seq: binary.LittleEndian.Uint32(data[off : off+4]),
			Throttle: math.Float32frombits(
				binary.LittleEndian.Uint32(data[off+4 : off+8]),
			),
			Brake: math.Float32frombits(
				binary.LittleEndian.Uint32(data[off+8 : off+12]),
			),
			Steer: math.Float32frombits(
				binary.LittleEndian.Uint32(data[off+12 : off+16]),
			),
			Gear:      int8(data[off+16]),
			Handbrake: data[off+17],
		}
	}
	return records
}

// hashStateStream replays the input records through physics.Step and hashes
// every replayHz-th State, returning a SHA-256 digest.
//
// SYNC NOTE: this function MUST stay byte-for-byte identical to replayAndHash
// in cmd/replaygen/main.go.
func hashStateStream(records []replayRecord) [32]byte {
	c := physics.DefaultConstants
	s := physics.State{
		Position:    [3]float32{0, suspGroundY, 0},
		Gear:        1,
		Orientation: [4]float32{0, 0, 0, 1},
	}
	h := sha256.New()
	scratch := make([]byte, 4)

	writeF32 := func(v float32) {
		binary.LittleEndian.PutUint32(scratch, math.Float32bits(v))
		h.Write(scratch)
	}
	writeU8 := func(v uint8) {
		scratch[0] = v
		h.Write(scratch[:1])
	}

	for i, r := range records {
		in := physics.Input{
			Throttle:  r.Throttle,
			Brake:     r.Brake,
			Steer:     r.Steer,
			Gear:      r.Gear,
			Handbrake: r.Handbrake != 0,
		}
		s = physics.Step(s, in, c, physics.FlatGround(0), replayDT)

		if (i+1)%replayHz == 0 {
			writeF32(s.Position[0])
			writeF32(s.Position[1])
			writeF32(s.Position[2])
			writeF32(s.Orientation[0])
			writeF32(s.Orientation[1])
			writeF32(s.Orientation[2])
			writeF32(s.Orientation[3])
			writeF32(s.LinearVel[0])
			writeF32(s.LinearVel[1])
			writeF32(s.LinearVel[2])
			writeF32(s.AngularVel[0])
			writeF32(s.AngularVel[1])
			writeF32(s.AngularVel[2])
			writeF32(s.RPM)
			writeU8(uint8(s.Gear))
			writeF32(s.WheelLoad[0])
			writeF32(s.WheelLoad[1])
			writeF32(s.WheelLoad[2])
			writeF32(s.WheelLoad[3])
			writeF32(s.WheelSlip[0])
			writeF32(s.WheelSlip[1])
			writeF32(s.WheelSlip[2])
			writeF32(s.WheelSlip[3])
			writeU8(s.Grounded)
		}
	}

	var digest [32]byte
	copy(digest[:], h.Sum(nil))
	return digest
}

// replayStates replays input records and returns all sampled State values
// (every replayHz-th step). Used by structural_invariants.
func replayStates(records []replayRecord) []physics.State {
	c := physics.DefaultConstants
	s := physics.State{
		Position:    [3]float32{0, suspGroundY, 0},
		Gear:        1,
		Orientation: [4]float32{0, 0, 0, 1},
	}
	sampled := make([]physics.State, 0, len(records)/replayHz+1)
	for i, r := range records {
		in := physics.Input{
			Throttle:  r.Throttle,
			Brake:     r.Brake,
			Steer:     r.Steer,
			Gear:      r.Gear,
			Handbrake: r.Handbrake != 0,
		}
		s = physics.Step(s, in, c, physics.FlatGround(0), replayDT)
		if (i+1)%replayHz == 0 {
			sampled = append(sampled, s)
		}
	}
	return sampled
}

// TestReplayDeterminism is the top-level determinism guard.
func TestReplayDeterminism(t *testing.T) {
	records := loadFixture(t)

	t.Run("golden_hash", func(t *testing.T) {
		// The Go math package does NOT guarantee bit-identical results across
		// architectures ("This package does not guarantee bit-identical results
		// across architectures."). The canonical platform is linux/amd64.
		if runtime.GOARCH != "amd64" {
			t.Skipf(
				"golden_hash skipped on %s: Go math is not bit-identical across "+
					"architectures (canonical platform is linux/amd64); "+
					"structural_invariants runs on all arches",
				runtime.GOARCH,
			)
		}

		goldenRaw, err := os.ReadFile(goldenFile)
		if err != nil {
			t.Fatalf("read golden: %v", err)
		}
		want := strings.TrimSpace(string(goldenRaw))

		digest := hashStateStream(records)
		got := fmt.Sprintf("%x", digest)

		if got != want {
			t.Errorf(
				"state-stream hash mismatch\n  got:  %s\n  want: %s\n"+
					"If physics constants were intentionally changed, "+
					"regenerate with: go run ./cmd/replaygen",
				got, want,
			)
		}
	})

	t.Run("structural_invariants", func(t *testing.T) {
		c := physics.DefaultConstants
		states := replayStates(records)
		if len(states) == 0 {
			t.Fatal("no sampled states — fixture may be empty")
		}
		for idx, s := range states {
			label := func(field string) string {
				return fmt.Sprintf("tick-sample %d field %s", idx, field)
			}
			checkFiniteF32 := func(name string, v float32) {
				if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
					t.Errorf("%s = %v, want finite", label(name), v)
				}
			}

			// Position, LinearVel, AngularVel must be finite.
			for i, v := range s.Position {
				checkFiniteF32(fmt.Sprintf("Position[%d]", i), v)
			}
			for i, v := range s.LinearVel {
				checkFiniteF32(fmt.Sprintf("LinearVel[%d]", i), v)
			}
			for i, v := range s.AngularVel {
				checkFiniteF32(fmt.Sprintf("AngularVel[%d]", i), v)
			}
			checkFiniteF32("RPM", s.RPM)

			// Orientation must be a unit quaternion within 1e-5.
			qx, qy, qz, qw := s.Orientation[0], s.Orientation[1],
				s.Orientation[2], s.Orientation[3]
			mag := math.Sqrt(float64(qx*qx + qy*qy + qz*qz + qw*qw))
			const quatTol = 1e-5
			if math.Abs(mag-1.0) > quatTol {
				t.Errorf(
					"%s: quaternion magnitude = %.8f, want 1.0 ± %.0e",
					label("Orientation"), mag, quatTol,
				)
			}

			// WheelLoad must be non-negative.
			for i, wl := range s.WheelLoad {
				if wl < 0 {
					t.Errorf(
						"%s = %.4f, want >= 0",
						label(fmt.Sprintf("WheelLoad[%d]", i)),
						wl,
					)
				}
				checkFiniteF32(fmt.Sprintf("WheelLoad[%d]", i), wl)
			}

			// RPM within [IdleRPM, RedlineRPM].
			if s.RPM < c.IdleRPM || s.RPM > c.RedlineRPM {
				t.Errorf(
					"%s = %.1f, want [%.1f, %.1f]",
					label("RPM"), s.RPM, c.IdleRPM, c.RedlineRPM,
				)
			}
		}
	})
}

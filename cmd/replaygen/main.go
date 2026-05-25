// cmd/replaygen generates the deterministic replay fixture and golden hash used
// by TestReplayDeterminism in internal/physics/replay_test.go.
//
// # Fixture record layout (20 bytes, little-endian)
//
//	[0:4]   Seq       uint32
//	[4:8]   Throttle  float32
//	[8:12]  Brake     float32
//	[12:16] Steer     float32
//	[16]    Gear      int8  (stored as byte)
//	[17]    Handbrake uint8
//	[18:20] _         padding (zero)
//
// Note: the plan interface comment says "16 bytes" but the field layout
// (4+4+4+4+1+1+2) is 20 bytes. We use 20. The test reads the same layout.
//
// # Hash routine
//
// MUST remain byte-for-byte identical to hashStateStream in
// internal/physics/replay_test.go. Samples every hz ticks (1 Hz), hashing
// State fields in canonical order: Position[0..2], Orientation[0..3],
// LinearVel[0..2], AngularVel[0..2], RPM, Gear, WheelLoad[0..3],
// WheelSlip[0..3], Grounded. Each float32 via binary.LittleEndian over
// math.Float32bits; each uint8/int8 as a single byte.
package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"math"
	"os"

	"github.com/JoakimCarlsson/sim-racing/internal/physics"
)

// recordSize is the number of bytes per fixture record.
const recordSize = 20

func main() {
	outBin := flag.String("out", "internal/physics/testdata/lap_inputs.bin",
		"path to write fixture binary")
	outGolden := flag.String(
		"golden",
		"internal/physics/testdata/lap_inputs.golden",
		"path to write golden hex digest",
	)
	seconds := flag.Int("seconds", 30, "duration of the scenario in seconds")
	hz := flag.Int("hz", 60, "simulation frequency in Hz")
	flag.Parse()

	totalTicks := *seconds * *hz
	dt := float32(1.0) / float32(*hz)

	// Build deterministic input script (no math/rand).
	inputs := buildInputScript(totalTicks)

	// Write fixture binary.
	if err := writeFixture(*outBin, inputs); err != nil {
		log.Fatalf("write fixture: %v", err)
	}
	fmt.Printf("wrote %d records (%d bytes) to %s\n",
		len(inputs), len(inputs)*recordSize, *outBin)

	// Replay through physics.Step and hash state stream.
	digest := replayAndHash(inputs, dt, *hz)
	hexStr := hex.EncodeToString(digest[:])

	// Write golden.
	if err := os.WriteFile(*outGolden, []byte(hexStr+"\n"), 0o644); err != nil {
		log.Fatalf("write golden: %v", err)
	}
	fmt.Printf("golden digest: %s\n", hexStr)
	fmt.Printf("wrote golden to %s\n", *outGolden)
}

// inputRecord is one record in the binary fixture file.
type inputRecord struct {
	Seq       uint32
	Throttle  float32
	Brake     float32
	Steer     float32
	Gear      int8
	Handbrake uint8
}

// buildInputScript produces a deterministic sequence of driver inputs using
// hand-coded scenario phases keyed on tick index. No math/rand is used.
//
// Phase schedule (at 60 Hz default):
//
//	   0– 299  (0.0–4.9 s)  standing start, full throttle, auto-shift gear 1→3
//	 300– 599  (5.0–9.9 s)  lift to 80 % throttle, hold gear 2
//	 600– 899 (10.0–14.9 s) partial brake + left steer, gear 2
//	 900–1199 (15.0–19.9 s) full throttle, gentle right steer ramp, gear 3
//	1200–1499 (20.0–24.9 s) heavy brake + left steer, gear 2
//	1500–1799 (25.0–29.9 s) acceleration out, straight, gear 3
func buildInputScript(totalTicks int) []inputRecord {
	records := make([]inputRecord, totalTicks)
	for i := 0; i < totalTicks; i++ {
		var r inputRecord
		r.Seq = uint32(i)
		switch {
		case i < 300:
			r.Throttle = 1.0
			r.Gear = gearForTick(i)
		case i < 600:
			r.Throttle = 0.8
			r.Gear = 2
		case i < 900:
			r.Throttle = 0.1
			r.Brake = 0.5
			r.Steer = -0.4
			r.Gear = 2
		case i < 1200:
			r.Throttle = 1.0
			r.Steer = steerRamp(i, 900, 1200, 0.0, 0.3)
			r.Gear = 3
		case i < 1500:
			r.Brake = 0.8
			r.Steer = -0.5
			r.Gear = 2
		default:
			r.Throttle = 0.9
			r.Gear = 3
		}
		records[i] = r
	}
	return records
}

// gearForTick returns a simple tick-based gear for the initial acceleration
// phase (deterministic, no rand).
func gearForTick(tick int) int8 {
	switch {
	case tick < 60:
		return 1
	case tick < 150:
		return 2
	default:
		return 3
	}
}

// steerRamp linearly interpolates steer from `from` to `to` over the tick
// range [tickStart, tickEnd).
func steerRamp(tick, tickStart, tickEnd int, from, to float32) float32 {
	if tick <= tickStart {
		return from
	}
	if tick >= tickEnd {
		return to
	}
	t := float32(tick-tickStart) / float32(tickEnd-tickStart)
	return from + t*(to-from)
}

// writeFixture serialises records to path as little-endian 20-byte packets.
func writeFixture(path string, records []inputRecord) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	buf := make([]byte, recordSize)
	for _, r := range records {
		binary.LittleEndian.PutUint32(buf[0:4], r.Seq)
		binary.LittleEndian.PutUint32(buf[4:8], math.Float32bits(r.Throttle))
		binary.LittleEndian.PutUint32(buf[8:12], math.Float32bits(r.Brake))
		binary.LittleEndian.PutUint32(buf[12:16], math.Float32bits(r.Steer))
		buf[16] = byte(r.Gear)
		buf[17] = r.Handbrake
		buf[18] = 0
		buf[19] = 0
		if _, werr := f.Write(buf); werr != nil {
			return werr
		}
	}
	return nil
}

// replayAndHash replays inputs through physics.Step using DefaultConstants and
// hashes the state stream, sampling every hz ticks (1 Hz).
//
// SYNC NOTE: This function MUST remain byte-for-byte identical to
// hashStateStream in internal/physics/replay_test.go. If you change field
// order or serialisation here, update that file too (and vice versa).
// suspEquilY computes the suspension-equilibrium CoM height (world Y) for c
// on FlatGround(0). At this height vertAccel = 0 and the car is at rest.
//
//	Y_eq = cgH + restLen - mass*g / (4*k)
func suspEquilY(c physics.Constants) float32 {
	const g = float32(9.81)
	k := c.SuspensionSpringK
	if k <= 0 {
		k = 40000
	}
	return c.CGHeight + c.SuspensionRestLength - c.Mass*g/(4*k)
}

func replayAndHash(records []inputRecord, dt float32, hz int) [32]byte {
	c := physics.DefaultConstants
	s := physics.State{
		Position:    [3]float32{0, suspEquilY(c), 0},
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
		s = physics.Step(s, in, c, physics.FlatGround(0), dt)

		// Sample every hz ticks (at ticks hz-1, 2*hz-1, … → 1 Hz sampling).
		if (i+1)%hz == 0 {
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

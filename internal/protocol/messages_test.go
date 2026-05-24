package protocol_test

import (
	"errors"
	"os"
	"testing"

	"github.com/JoakimCarlsson/sim-racing/internal/physics"
	"github.com/JoakimCarlsson/sim-racing/internal/protocol"
)

// ---- fixtures ---------------------------------------------------------------

// fixedClientInput returns a deterministic ClientInput for golden tests.
func fixedClientInput() protocol.ClientInput {
	return protocol.ClientInput{
		Seq:          1,
		ClientTimeMs: 1000,
		Input: physics.Input{
			Throttle:  0.75,
			Brake:     0.0,
			Steer:     -0.5,
			Gear:      2,
			Handbrake: false,
		},
	}
}

// fixedCarState returns a deterministic CarState with every physics.State
// field set to a distinct non-zero sentinel.
func fixedCarState() protocol.CarState {
	var nick [8]byte
	copy(nick[:], "player01")
	return protocol.CarState{
		PlayerID: 7,
		Nickname: nick,
		State: physics.State{
			Position:    [3]float32{1.0, 2.0, 3.0},
			Orientation: [4]float32{0.0, 0.0, 0.0, 1.0},
			LinearVel:   [3]float32{10.0, 0.0, 20.0},
			AngularVel:  [3]float32{0.1, 0.2, 0.3},
			RPM:         3500.0,
			Gear:        3,
			WheelLoad:   [4]float32{2500.0, 2500.0, 2700.0, 2700.0},
			WheelSlip:   [4]float32{0.05, 0.05, 0.1, 0.1},
			Grounded:    0x0F,
		},
	}
}

// fixedServerSnapshot returns a deterministic ServerSnapshot.
func fixedServerSnapshot() protocol.ServerSnapshot {
	return protocol.ServerSnapshot{
		ServerTickMs: 5000,
		LastAckedSeq: 42,
		Cars:         []protocol.CarState{fixedCarState()},
	}
}

// fixedServerHello returns a deterministic ServerHello using DefaultConstants.
func fixedServerHello() protocol.ServerHello {
	return protocol.ServerHello{
		PlayerID:     7,
		ServerTickHz: 60,
		SnapshotHz:   20,
		Constants:    physics.DefaultConstants,
	}
}

// ---- round-trip tests -------------------------------------------------------

func TestRoundTrip_ClientInput(t *testing.T) {
	orig := fixedClientInput()

	buf := make([]byte, protocol.ClientInputSize)
	n, err := orig.Marshal(buf)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if n != protocol.ClientInputSize {
		t.Fatalf("Marshal: want %d bytes, got %d", protocol.ClientInputSize, n)
	}
	if buf[0] != byte(protocol.MsgClientInput) {
		t.Errorf("byte[0] want 0x%02x, got 0x%02x",
			byte(protocol.MsgClientInput), buf[0])
	}
	if buf[1] != protocol.ProtocolVersion {
		t.Errorf("byte[1] want %d, got %d", protocol.ProtocolVersion, buf[1])
	}

	var got protocol.ClientInput
	if err := got.Unmarshal(buf[:n]); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got != orig {
		t.Errorf("round-trip mismatch:\n  want %+v\n  got  %+v", orig, got)
	}
}

func TestRoundTrip_ServerSnapshot(t *testing.T) {
	orig := fixedServerSnapshot()

	buf := make([]byte, protocol.ServerSnapshotSize(len(orig.Cars)))
	n, err := orig.Marshal(buf)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if buf[0] != byte(protocol.MsgServerSnapshot) {
		t.Errorf("byte[0] want 0x%02x, got 0x%02x",
			byte(protocol.MsgServerSnapshot), buf[0])
	}
	if buf[1] != protocol.ProtocolVersion {
		t.Errorf("byte[1] want %d, got %d", protocol.ProtocolVersion, buf[1])
	}

	got := protocol.ServerSnapshot{Cars: make([]protocol.CarState, 1)}
	if err := got.Unmarshal(buf[:n]); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.ServerTickMs != orig.ServerTickMs {
		t.Errorf(
			"ServerTickMs: want %d, got %d",
			orig.ServerTickMs,
			got.ServerTickMs,
		)
	}
	if got.LastAckedSeq != orig.LastAckedSeq {
		t.Errorf(
			"LastAckedSeq: want %d, got %d",
			orig.LastAckedSeq,
			got.LastAckedSeq,
		)
	}
	if len(got.Cars) != len(orig.Cars) {
		t.Fatalf("Cars len: want %d, got %d", len(orig.Cars), len(got.Cars))
	}
	if got.Cars[0] != orig.Cars[0] {
		t.Errorf("Cars[0] round-trip mismatch:\n  want %+v\n  got  %+v",
			orig.Cars[0], got.Cars[0])
	}
}

func TestRoundTrip_ServerHello(t *testing.T) {
	orig := fixedServerHello()

	n := protocol.ServerHelloSize(&orig.Constants)
	buf := make([]byte, n)
	written, err := orig.Marshal(buf)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if written != n {
		t.Fatalf("Marshal: want %d bytes, got %d", n, written)
	}
	if buf[0] != byte(protocol.MsgServerHello) {
		t.Errorf("byte[0] want 0x%02x, got 0x%02x",
			byte(protocol.MsgServerHello), buf[0])
	}
	if buf[1] != protocol.ProtocolVersion {
		t.Errorf("byte[1] want %d, got %d", protocol.ProtocolVersion, buf[1])
	}

	var got protocol.ServerHello
	if err := got.Unmarshal(buf[:written]); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.PlayerID != orig.PlayerID {
		t.Errorf("PlayerID: want %d, got %d", orig.PlayerID, got.PlayerID)
	}
	if got.ServerTickHz != orig.ServerTickHz {
		t.Errorf(
			"ServerTickHz: want %d, got %d",
			orig.ServerTickHz,
			got.ServerTickHz,
		)
	}
	if got.SnapshotHz != orig.SnapshotHz {
		t.Errorf("SnapshotHz: want %d, got %d", orig.SnapshotHz, got.SnapshotHz)
	}
	c := orig.Constants
	g := got.Constants
	if c.Mass != g.Mass {
		t.Errorf("Constants.Mass: want %v, got %v", c.Mass, g.Mass)
	}
	if c.RedlineRPM != g.RedlineRPM {
		t.Errorf(
			"Constants.RedlineRPM: want %v, got %v",
			c.RedlineRPM,
			g.RedlineRPM,
		)
	}
	if len(c.GearRatios) != len(g.GearRatios) {
		t.Fatalf(
			"GearRatios len: want %d, got %d",
			len(c.GearRatios),
			len(g.GearRatios),
		)
	}
	for i, r := range c.GearRatios {
		if r != g.GearRatios[i] {
			t.Errorf("GearRatios[%d]: want %v, got %v", i, r, g.GearRatios[i])
		}
	}
}

// ---- sentinel error tests ---------------------------------------------------

func TestUnmarshal_UnknownMsgType(t *testing.T) {
	buf := make([]byte, protocol.ClientInputSize)
	buf[0] = 0xFF // unknown type
	buf[1] = protocol.ProtocolVersion

	var ci protocol.ClientInput
	err := ci.Unmarshal(buf)
	if !errors.Is(err, protocol.ErrUnknownMsgType) {
		t.Errorf("want ErrUnknownMsgType, got %v", err)
	}
}

func TestUnmarshal_UnsupportedVersion(t *testing.T) {
	buf := make([]byte, protocol.ClientInputSize)
	buf[0] = byte(protocol.MsgClientInput)
	buf[1] = 0xFF // unsupported version

	var ci protocol.ClientInput
	err := ci.Unmarshal(buf)
	if !errors.Is(err, protocol.ErrUnsupportedVersion) {
		t.Errorf("want ErrUnsupportedVersion, got %v", err)
	}
}

func TestUnmarshal_ShortBuffer(t *testing.T) {
	buf := []byte{byte(protocol.MsgClientInput), protocol.ProtocolVersion}
	// only 2 bytes — too short for ClientInput

	var ci protocol.ClientInput
	err := ci.Unmarshal(buf)
	if !errors.Is(err, protocol.ErrShortBuffer) {
		t.Errorf("want ErrShortBuffer, got %v", err)
	}
}

// ---- golden bytes test ------------------------------------------------------

func TestGoldenBytes(t *testing.T) {
	t.Run("ClientInput", func(t *testing.T) {
		orig := fixedClientInput()
		buf := make([]byte, protocol.ClientInputSize)
		n, err := orig.Marshal(buf)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		golden, err := os.ReadFile(
			"testdata/golden_client_input.bin",
		)
		if err != nil {
			t.Fatalf("read golden: %v", err)
		}
		if n != len(golden) {
			t.Fatalf("length: want %d, got %d", len(golden), n)
		}
		for i := range golden {
			if buf[i] != golden[i] {
				t.Errorf("byte[%d]: want 0x%02x, got 0x%02x",
					i, golden[i], buf[i])
			}
		}
	})

	t.Run("ServerSnapshot", func(t *testing.T) {
		orig := fixedServerSnapshot()
		buf := make([]byte, protocol.ServerSnapshotSize(len(orig.Cars)))
		n, err := orig.Marshal(buf)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		golden, err := os.ReadFile(
			"testdata/golden_server_snapshot.bin",
		)
		if err != nil {
			t.Fatalf("read golden: %v", err)
		}
		if n != len(golden) {
			t.Fatalf("length: want %d, got %d", len(golden), n)
		}
		for i := range golden {
			if buf[i] != golden[i] {
				t.Errorf("byte[%d]: want 0x%02x, got 0x%02x",
					i, golden[i], buf[i])
			}
		}
	})

	t.Run("ServerHello", func(t *testing.T) {
		orig := fixedServerHello()
		size := protocol.ServerHelloSize(&orig.Constants)
		buf := make([]byte, size)
		n, err := orig.Marshal(buf)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		golden, err := os.ReadFile(
			"testdata/golden_server_hello.bin",
		)
		if err != nil {
			t.Fatalf("read golden: %v", err)
		}
		if n != len(golden) {
			t.Fatalf("length: want %d, got %d", len(golden), n)
		}
		for i := range golden {
			if buf[i] != golden[i] {
				t.Errorf("byte[%d]: want 0x%02x, got 0x%02x",
					i, golden[i], buf[i])
			}
		}
	})
}

// ---- zero-alloc test --------------------------------------------------------

func TestZeroAllocs(t *testing.T) {
	ci := fixedClientInput()
	ciBuf := make([]byte, protocol.ClientInputSize)

	allocs := testing.AllocsPerRun(1000, func() {
		_, _ = ci.Marshal(ciBuf)
		var got protocol.ClientInput
		_ = got.Unmarshal(ciBuf)
	})
	if allocs != 0 {
		t.Errorf("ClientInput Marshal+Unmarshal: want 0 allocs, got %v", allocs)
	}

	snap := protocol.ServerSnapshot{
		ServerTickMs: 1,
		LastAckedSeq: 0,
		Cars:         make([]protocol.CarState, 1),
	}
	snap.Cars[0] = fixedCarState()
	snapBuf := make([]byte, protocol.ServerSnapshotSize(1))
	preAllocedCars := make([]protocol.CarState, 1)

	snapAllocs := testing.AllocsPerRun(1000, func() {
		_, _ = snap.Marshal(snapBuf)
		got := protocol.ServerSnapshot{Cars: preAllocedCars}
		_ = got.Unmarshal(snapBuf)
	})
	if snapAllocs != 0 {
		t.Errorf("ServerSnapshot Marshal+Unmarshal: want 0 allocs, got %v",
			snapAllocs)
	}
}

// ---- all state fields round-trip test ---------------------------------------

func TestServerSnapshot_AllStateFieldsRoundTrip(t *testing.T) {
	orig := protocol.ServerSnapshot{
		ServerTickMs: 99,
		LastAckedSeq: 5,
		Cars: []protocol.CarState{
			{
				PlayerID: 3,
				Nickname: [8]byte{'t', 'e', 's', 't'},
				State: physics.State{
					Position:    [3]float32{1.1, 2.2, 3.3},
					Orientation: [4]float32{0.1, 0.2, 0.3, 0.9},
					LinearVel:   [3]float32{4.4, 5.5, 6.6},
					AngularVel:  [3]float32{7.7, 8.8, 9.9},
					RPM:         6000.0,
					Gear:        -1, // reverse — sentinel
					WheelLoad:   [4]float32{1111.0, 2222.0, 3333.0, 4444.0},
					WheelSlip:   [4]float32{0.11, 0.22, 0.33, 0.44},
					Grounded:    0x0F, // all wheels grounded
				},
			},
		},
	}

	buf := make([]byte, protocol.ServerSnapshotSize(1))
	n, err := orig.Marshal(buf)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	got := protocol.ServerSnapshot{Cars: make([]protocol.CarState, 1)}
	if err := got.Unmarshal(buf[:n]); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	s := got.Cars[0].State
	o := orig.Cars[0].State

	// Check every field explicitly.
	if s.Position != o.Position {
		t.Errorf("Position: want %v, got %v", o.Position, s.Position)
	}
	if s.Orientation != o.Orientation {
		t.Errorf("Orientation: want %v, got %v", o.Orientation, s.Orientation)
	}
	if s.LinearVel != o.LinearVel {
		t.Errorf("LinearVel: want %v, got %v", o.LinearVel, s.LinearVel)
	}
	if s.AngularVel != o.AngularVel {
		t.Errorf("AngularVel: want %v, got %v", o.AngularVel, s.AngularVel)
	}
	if s.RPM != o.RPM {
		t.Errorf("RPM: want %v, got %v", o.RPM, s.RPM)
	}
	if s.Gear != o.Gear {
		t.Errorf("Gear: want %v, got %v", o.Gear, s.Gear)
	}
	if s.WheelLoad != o.WheelLoad {
		t.Errorf("WheelLoad: want %v, got %v", o.WheelLoad, s.WheelLoad)
	}
	if s.WheelSlip != o.WheelSlip {
		t.Errorf("WheelSlip: want %v, got %v", o.WheelSlip, s.WheelSlip)
	}
	if s.Grounded != o.Grounded {
		t.Errorf("Grounded: want 0x%02x, got 0x%02x", o.Grounded, s.Grounded)
	}
}

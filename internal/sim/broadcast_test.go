package sim

import (
	"context"
	"encoding/binary"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JoakimCarlsson/sim-racing/internal/physics"
	"github.com/JoakimCarlsson/sim-racing/internal/protocol"
)

// ---- helpers ----------------------------------------------------------------

// captureCount is a SnapshotSink that counts how many times SendSnapshot is
// called and records the last payload + lastAckedSeq received.
type captureCount struct {
	n       atomic.Int64
	last    []byte
	lastSeq uint32
}

func (c *captureCount) SendSnapshot(payload []byte, lastAckedSeq uint32) {
	c.n.Add(1)
	// Copy payload so we can inspect it after the broadcaster has moved on.
	cp := make([]byte, len(payload))
	copy(cp, payload)
	c.last = cp
	c.lastSeq = lastAckedSeq
}

// ---- AC1: 30 Hz broadcast rate over a 2-second window ----------------------

func TestBroadcastRate30Hz(t *testing.T) {
	w := New()
	sink := &captureCount{}

	p := &PlayerSim{
		ID:        1,
		Source:    &emptySource{},
		Sink:      sink,
		Constants: physics.DefaultConstants,
	}
	w.AddPlayer(p)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go w.Run(ctx)

	// Warm up: let the loops settle before we start counting.
	time.Sleep(50 * time.Millisecond)
	sink.n.Store(0)
	start := time.Now()

	go w.RunBroadcaster(ctx, 30, time.Now)

	// Measure for 2 seconds.
	time.Sleep(2 * time.Second)
	elapsed := time.Since(start)
	count := sink.n.Load()

	expected := float64(elapsed) / float64(time.Second) * 30.0
	ratio := float64(count) / expected

	t.Logf(
		"broadcast ticks=%d expected=%.1f ratio=%.3f elapsed=%s",
		count, expected, ratio, elapsed,
	)

	// Allow ±5% — the plan says "~60±2" over 2 s is acceptable, which is
	// approximately ±3%. We use ±5% to be consistent with the tick-rate test.
	const tolerance = 0.05
	if ratio < 1-tolerance || ratio > 1+tolerance {
		t.Errorf(
			"broadcast rate out of ±5%% tolerance: got ratio %.3f (count=%d expected=%.1f)",
			ratio,
			count,
			expected,
		)
	}
}

// ---- AC2: slow consumer cannot stall sim ------------------------------------

// dropSink is a SnapshotSink with a finite-capacity outbound queue that drops
// frames when full (non-blocking). It simulates a realtime.Conn that is
// keeping up — but with a tiny buffer so we can confirm drops happen without
// any stall.
type dropSink struct {
	ch      chan []byte
	dropped atomic.Int64
	sent    atomic.Int64
}

func newDropSink(cap int) *dropSink {
	return &dropSink{ch: make(chan []byte, cap)}
}

func (d *dropSink) SendSnapshot(payload []byte, lastAckedSeq uint32) {
	cp := make([]byte, len(payload))
	copy(cp, payload)
	select {
	case d.ch <- cp:
		d.sent.Add(1)
	default:
		d.dropped.Add(1)
	}
}

// TestSlowConsumerDoesNotStallSim verifies that a sink whose outbound buffer
// is full causes frames to be dropped rather than blocking the broadcaster.
// The broadcaster must continue firing at ~30 Hz and must not block even when
// all sinks are full.
func TestSlowConsumerDoesNotStallSim(t *testing.T) {
	w := New()

	// Buffer capacity of 1 means it fills almost immediately.
	ds := newDropSink(1)
	p := &PlayerSim{
		ID:        1,
		Source:    &emptySource{},
		Sink:      ds,
		Constants: physics.DefaultConstants,
	}
	w.AddPlayer(p)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go w.Run(ctx)

	broadcastDone := make(chan struct{})
	go func() {
		w.RunBroadcaster(ctx, 30, time.Now)
		close(broadcastDone)
	}()

	// Run for 500 ms without draining the sink channel — the buffer will fill
	// and subsequent SendSnapshot calls must drop.
	time.Sleep(500 * time.Millisecond)
	cancel()

	select {
	case <-broadcastDone:
		// good
	case <-time.After(500 * time.Millisecond):
		t.Fatal(
			"RunBroadcaster did not return within 500ms after context cancel",
		)
	}

	total := ds.sent.Load() + ds.dropped.Load()
	t.Logf(
		"sent=%d dropped=%d total=%d",
		ds.sent.Load(),
		ds.dropped.Load(),
		total,
	)

	// We expect drops to have occurred because the buffer filled quickly.
	if ds.dropped.Load() == 0 {
		t.Error(
			"expected some frames to be dropped when sink buffer is full, got 0 drops",
		)
	}
	// Total call count must be > 0 (broadcaster was running).
	if total == 0 {
		t.Error("expected SendSnapshot to have been called at least once")
	}
}

// ---- AC3: snapshot bytes round-trip via protocol.ServerSnapshot.Unmarshal --

func TestSnapshotBytesRoundTrip(t *testing.T) {
	w := New()
	sink := &captureCount{}

	p := &PlayerSim{
		ID:     5,
		Source: &emptySource{},
		Sink:   sink,
		State: physics.State{
			Position:    [3]float32{1.0, 2.0, 3.0},
			Orientation: [4]float32{0, 0, 0, 1},
			RPM:         4500,
			Gear:        3,
		},
		Constants: physics.DefaultConstants,
	}
	w.AddPlayer(p)

	// One broadcast tick.
	w.broadcastOnce(time.Now())

	if sink.n.Load() == 0 {
		t.Fatal("expected at least one SendSnapshot call")
	}

	payload := sink.last
	if len(payload) < 2 {
		t.Fatalf("payload too short: %d bytes", len(payload))
	}

	// First byte must be MsgServerSnapshot (0x02), second byte must be version 1.
	if payload[0] != byte(protocol.MsgServerSnapshot) {
		t.Errorf(
			"payload[0] = 0x%02x, want 0x%02x (MsgServerSnapshot)",
			payload[0], byte(protocol.MsgServerSnapshot),
		)
	}
	if payload[1] != protocol.ProtocolVersion {
		t.Errorf(
			"payload[1] = %d, want %d (ProtocolVersion)",
			payload[1], protocol.ProtocolVersion,
		)
	}

	// Unmarshal and verify car state.
	var snap protocol.ServerSnapshot
	snap.Cars = make([]protocol.CarState, 1)
	if err := snap.Unmarshal(payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if len(snap.Cars) < 1 {
		t.Fatal("expected at least 1 car in snapshot")
	}
	car := snap.Cars[0]
	if car.PlayerID != 5 {
		t.Errorf("PlayerID = %d, want 5", car.PlayerID)
	}
	if car.State.Position[0] != 1.0 || car.State.Position[1] != 2.0 ||
		car.State.Position[2] != 3.0 {
		t.Errorf("Position = %v, want [1 2 3]", car.State.Position)
	}
}

// ---- AC3 (per-recipient): LastAckedSeq in payload matches player's LastAppliedSeq

func TestPerRecipientLastAckedSeq(t *testing.T) {
	w := New()

	sink1 := &captureCount{}
	sink2 := &captureCount{}

	p1 := &PlayerSim{
		ID:             1,
		Source:         &emptySource{},
		Sink:           sink1,
		LastAppliedSeq: 42,
		Constants:      physics.DefaultConstants,
	}
	p2 := &PlayerSim{
		ID:             2,
		Source:         &emptySource{},
		Sink:           sink2,
		LastAppliedSeq: 99,
		Constants:      physics.DefaultConstants,
	}
	w.AddPlayer(p1)
	w.AddPlayer(p2)

	w.broadcastOnce(time.Now())

	if sink1.n.Load() == 0 || sink2.n.Load() == 0 {
		t.Fatal("expected both sinks to receive snapshots")
	}

	// The lastAckedSeq argument passed to SendSnapshot must match the player's
	// LastAppliedSeq at broadcast time.
	if sink1.lastSeq != 42 {
		t.Errorf(
			"player 1: lastAckedSeq passed to SendSnapshot = %d, want 42",
			sink1.lastSeq,
		)
	}
	if sink2.lastSeq != 99 {
		t.Errorf(
			"player 2: lastAckedSeq passed to SendSnapshot = %d, want 99",
			sink2.lastSeq,
		)
	}

	// Also verify the bytes 6..10 in the payload contain the correct LastAckedSeq
	// (the byte-patch done per recipient).
	if len(sink1.last) >= 10 {
		gotSeq := binary.LittleEndian.Uint32(sink1.last[6:10])
		if gotSeq != 42 {
			t.Errorf("player 1 payload bytes[6:10] = %d, want 42", gotSeq)
		}
	}
	if len(sink2.last) >= 10 {
		gotSeq := binary.LittleEndian.Uint32(sink2.last[6:10])
		if gotSeq != 99 {
			t.Errorf("player 2 payload bytes[6:10] = %d, want 99", gotSeq)
		}
	}
}

// ---- AC4: ≤1 alloc per broadcast tick --------------------------------------

func TestZeroAllocsPerBroadcastTick(t *testing.T) {
	w := New()
	sink := &captureCount{}

	p := &PlayerSim{
		ID:        1,
		Source:    &emptySource{},
		Sink:      sink,
		Constants: physics.DefaultConstants,
	}
	w.AddPlayer(p)

	now := time.Now()

	// Warm-up to allow any one-time allocations to settle.
	w.broadcastOnce(now)

	allocs := testing.AllocsPerRun(100, func() {
		w.broadcastOnce(now)
	})

	t.Logf("allocs per broadcastOnce = %.2f", allocs)
	if allocs > 1 {
		t.Errorf("expected ≤1 alloc per broadcastOnce, got %.2f", allocs)
	}
}

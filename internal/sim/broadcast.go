package sim

import (
	"context"
	"encoding/binary"
	"math"
	"time"

	"github.com/JoakimCarlsson/sim-racing/internal/physics"
	"github.com/JoakimCarlsson/sim-racing/internal/protocol"
)

// SnapshotSink is satisfied by any connection that can receive a snapshot
// payload. internal/realtime.Conn satisfies this structurally.
//
// payload is the fully encoded ServerSnapshot frame. The slice is shared
// across recipients: the broadcaster patches bytes 6..10 (LastAckedSeq) for
// each recipient and then calls SendSnapshot. Implementations MUST copy the
// payload if they need to retain it past the call; they MUST NOT block.
//
// lastAckedSeq is the per-recipient value already patched into bytes 6..10.
type SnapshotSink interface {
	SendSnapshot(payload []byte, lastAckedSeq uint32)
}

// snapshotView is a minimal capture of a player's state taken while the
// world lock is held. Using a fixed-size pre-encoded state avoids carrying
// live physics.State pointers outside the lock.
type snapshotView struct {
	id             uint16
	encodedState   [stateWireBytes]byte
	lastAppliedSeq uint32
	sink           SnapshotSink
}

// stateWireBytes is the on-wire size of physics.State (90 bytes).
// Matches protocol.stateSize; duplicated here to avoid exporting a private const.
const stateWireBytes = 90

// broadcastBuf is the broadcaster's pre-allocated scratch space. It lives on
// *World so RunBroadcaster / broadcastOnce reuse the same memory every tick.
type broadcastBufT struct {
	// wire holds the serialised ServerSnapshot frame.  Grows but never shrinks.
	wire []byte
	// views is reused each tick; capacity grows but slice is reset to len 0.
	views []snapshotView
}

// broadcastOnce encodes one snapshot frame and calls SendSnapshot on every
// player's sink with a per-recipient LastAckedSeq patch.
//
// This is the unit-testable inner loop; RunBroadcaster drives it on a timer.
func (w *World) broadcastOnce(now time.Time) {
	// --- collect a lightweight view of each player (under lock) -----------
	w.mu.Lock()
	n := len(w.players)
	if cap(w.bcastBuf.views) < n {
		w.bcastBuf.views = make([]snapshotView, 0, n)
	}
	w.bcastBuf.views = w.bcastBuf.views[:0]

	for _, p := range w.players {
		if p.Sink == nil {
			continue
		}
		var sv snapshotView
		sv.id = p.ID
		sv.lastAppliedSeq = p.LastAppliedSeq
		sv.sink = p.Sink
		marshalPhysicsState(&sv.encodedState, &p.State)
		w.bcastBuf.views = append(w.bcastBuf.views, sv)
	}
	w.mu.Unlock()

	views := w.bcastBuf.views
	if len(views) == 0 {
		return
	}

	// --- size the shared wire buffer --------------------------------------
	need := protocol.ServerSnapshotSize(len(views))
	if len(w.bcastBuf.wire) < need {
		w.bcastBuf.wire = make([]byte, need)
	}
	wire := w.bcastBuf.wire[:need]

	// --- encode base frame (LastAckedSeq placeholder = 0) ----------------
	tickMs := uint32(now.UnixMilli())
	encodeSnapshotFrame(wire, tickMs, views)

	// --- per-recipient delivery with LastAckedSeq patch ------------------
	for i := range views {
		sv := &views[i]
		binary.LittleEndian.PutUint32(wire[6:], sv.lastAppliedSeq)
		sv.sink.SendSnapshot(wire, sv.lastAppliedSeq)
	}
}

// encodeSnapshotFrame writes a complete ServerSnapshot binary frame into dst.
// LastAckedSeq is written as 0 at offset 6; callers patch it per-recipient.
// dst must be at least protocol.ServerSnapshotSize(len(views)) bytes.
func encodeSnapshotFrame(dst []byte, tickMs uint32, views []snapshotView) {
	dst[0] = byte(protocol.MsgServerSnapshot)
	dst[1] = protocol.ProtocolVersion
	binary.LittleEndian.PutUint32(dst[2:], tickMs)
	binary.LittleEndian.PutUint32(
		dst[6:],
		0,
	) // placeholder, patched per-recipient
	dst[10] = uint8(len(views))

	off := 11 // serverSnapshotHeaderSize (2+4+4+1)
	for i := range views {
		sv := &views[i]
		binary.LittleEndian.PutUint16(dst[off:], sv.id)
		off += 2
		// Nickname: zero-filled [8]byte (nickname population is a non-goal).
		for j := 0; j < 8; j++ {
			dst[off+j] = 0
		}
		off += 8
		copy(dst[off:], sv.encodedState[:])
		off += stateWireBytes
	}
}

// marshalPhysicsState encodes s into a fixed 90-byte little-endian array
// using the same layout as protocol.marshalState (docs/protocol.md).
func marshalPhysicsState(dst *[stateWireBytes]byte, s *physics.State) {
	off := 0
	putF32b(dst, &off, s.Position[0])
	putF32b(dst, &off, s.Position[1])
	putF32b(dst, &off, s.Position[2])
	putF32b(dst, &off, s.Orientation[0])
	putF32b(dst, &off, s.Orientation[1])
	putF32b(dst, &off, s.Orientation[2])
	putF32b(dst, &off, s.Orientation[3])
	putF32b(dst, &off, s.LinearVel[0])
	putF32b(dst, &off, s.LinearVel[1])
	putF32b(dst, &off, s.LinearVel[2])
	putF32b(dst, &off, s.AngularVel[0])
	putF32b(dst, &off, s.AngularVel[1])
	putF32b(dst, &off, s.AngularVel[2])
	putF32b(dst, &off, s.RPM)
	dst[off] = uint8(s.Gear)
	off++
	putF32b(dst, &off, s.WheelLoad[0])
	putF32b(dst, &off, s.WheelLoad[1])
	putF32b(dst, &off, s.WheelLoad[2])
	putF32b(dst, &off, s.WheelLoad[3])
	putF32b(dst, &off, s.WheelSlip[0])
	putF32b(dst, &off, s.WheelSlip[1])
	putF32b(dst, &off, s.WheelSlip[2])
	putF32b(dst, &off, s.WheelSlip[3])
	dst[off] = s.Grounded
}

// putF32b writes a float32 at dst[*off] (little-endian) and advances *off by 4.
func putF32b(dst *[stateWireBytes]byte, off *int, v float32) {
	binary.LittleEndian.PutUint32(dst[*off:], math.Float32bits(v))
	*off += 4
}

// RunBroadcaster runs the snapshot broadcast loop at hz per second until ctx
// is cancelled. nowFn is called each tick to populate ServerTickMs (pass
// time.Now in production).
//
// Run this in its own goroutine; it blocks until ctx is done.
func (w *World) RunBroadcaster(
	ctx context.Context,
	hz int,
	nowFn func() time.Time,
) {
	interval := time.Duration(float64(time.Second) / float64(hz))
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.broadcastOnce(nowFn())
		}
	}
}

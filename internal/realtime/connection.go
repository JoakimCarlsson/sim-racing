// Package realtime owns WebSocket connection lifecycle for sim-racing clients.
package realtime

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/JoakimCarlsson/sim-racing/internal/protocol"
	"github.com/coder/websocket"
)

// ringCap is the fixed capacity of the per-connection input ring buffer.
// Overflow policy: drop-oldest (the simulator always wants the freshest
// input; a stale input from a lagging sender is less useful than nothing).
const ringCap = 128

// defaultMaxGear is the maximum forward gear used when Conn.MaxGear is zero.
const defaultMaxGear int8 = 6

// defaultMinInputIntervalNs is the minimum wall-clock gap (ns) between two
// accepted inputs (≈ 125 Hz ceiling).  Used when Conn.MinInputIntervalNs is
// zero.
const defaultMinInputIntervalNs int64 = 8_000_000 // 8 ms

// defaultInvalidThreshold is the number of bad frames within InvalidWindow
// before the connection is forcibly closed.  Used when Conn.InvalidThreshold
// is zero.
const defaultInvalidThreshold uint32 = 32

// defaultInvalidWindow is the rolling window over which InvalidThreshold is
// counted.  Used when Conn.InvalidWindow is zero.
const defaultInvalidWindow = time.Second

// Conn wraps a single WebSocket connection and its lifecycle callbacks.
type Conn struct {
	// ID is a monotonically increasing identifier assigned by the HTTP layer.
	ID uint64

	// WS is the accepted WebSocket connection.
	WS *websocket.Conn

	// MaxGear is the highest forward gear (default 6).
	// Reverse is always -1; neutral is 0.
	MaxGear int8

	// MinInputIntervalNs is the minimum nanoseconds between accepted inputs
	// (default 8_000_000 = 8 ms, i.e. a ≈125 Hz ceiling).
	MinInputIntervalNs int64

	// InvalidThreshold is the maximum number of invalid frames allowed within
	// InvalidWindow before the connection is closed (default 32).
	InvalidThreshold uint32

	// InvalidWindow is the rolling window for InvalidThreshold (default 1s).
	InvalidWindow time.Duration

	// InvalidReasonLog is called each time an invalid frame is received.
	// May be nil.
	InvalidReasonLog func(id uint64, reason InvalidReason, err error)

	// OnOpen is called once immediately before the read loop starts.
	// May be nil.
	OnOpen func(id uint64)

	// OnClose is called once after the read loop exits, with the error that
	// caused it to exit (nil on a clean close).
	// May be nil.
	OnClose func(id uint64, err error)

	// --- ring buffer (guarded by ringMu) ---
	ringMu   sync.Mutex
	ring     [ringCap]protocol.ClientInput
	ringHead int // index of oldest entry
	ringLen  int // number of valid entries (0..ringCap)

	// --- invalid frame counter ---
	invalidCount atomic.Uint32
}

// InvalidCount returns the total number of invalid frames received since the
// connection was opened.
func (c *Conn) InvalidCount() uint32 {
	return c.invalidCount.Load()
}

// DrainInputs copies up to len(dst) buffered inputs (oldest-first) into dst
// and removes them from the ring.  Returns the number of entries written.
// Safe to call from any goroutine.
func (c *Conn) DrainInputs(dst []protocol.ClientInput) int {
	c.ringMu.Lock()
	defer c.ringMu.Unlock()

	n := c.ringLen
	if n > len(dst) {
		n = len(dst)
	}
	for i := 0; i < n; i++ {
		dst[i] = c.ring[(c.ringHead+i)%ringCap]
	}
	c.ringHead = (c.ringHead + n) % ringCap
	c.ringLen -= n
	return n
}

// enqueue adds in to the ring buffer.  If the buffer is full, the oldest
// entry is overwritten (drop-oldest policy).
// Must be called with ringMu held.
func (c *Conn) enqueue(in protocol.ClientInput) {
	tail := (c.ringHead + c.ringLen) % ringCap
	if c.ringLen == ringCap {
		// Drop-oldest: advance head, overwrite the slot it vacated.
		c.ring[c.ringHead] = in
		c.ringHead = (c.ringHead + 1) % ringCap
		// ringLen stays ringCap.
	} else {
		c.ring[tail] = in
		c.ringLen++
	}
}

// effectiveMaxGear returns MaxGear falling back to the default.
func (c *Conn) effectiveMaxGear() int8 {
	if c.MaxGear == 0 {
		return defaultMaxGear
	}
	return c.MaxGear
}

// effectiveMinIntervalNs returns MinInputIntervalNs falling back to the default.
func (c *Conn) effectiveMinIntervalNs() int64 {
	if c.MinInputIntervalNs == 0 {
		return defaultMinInputIntervalNs
	}
	return c.MinInputIntervalNs
}

// effectiveInvalidThreshold returns InvalidThreshold falling back to the default.
func (c *Conn) effectiveInvalidThreshold() uint32 {
	if c.InvalidThreshold == 0 {
		return defaultInvalidThreshold
	}
	return c.InvalidThreshold
}

// effectiveInvalidWindow returns InvalidWindow falling back to the default.
func (c *Conn) effectiveInvalidWindow() time.Duration {
	if c.InvalidWindow == 0 {
		return defaultInvalidWindow
	}
	return c.InvalidWindow
}

// Serve runs the connection's read loop: it calls OnOpen, reads binary frames
// containing ClientInput messages (0x01), runs sanity gates, and enqueues
// valid inputs to the per-connection 128-slot ring buffer (drop-oldest policy).
//
// Text frames are still echoed for backward compatibility with tests that rely
// on the echo behaviour.
//
// A connection is forcibly closed when it accumulates more than
// InvalidThreshold invalid frames within InvalidWindow.
//
// The WebSocket upgrade layer (426 Upgrade Required) is handled by the HTTP
// layer in internal/http/realtime_endpoint.go; any error there is already
// written to the response before Serve is called.
//
// Serve blocks until the connection is closed or ctx is cancelled.
func (c *Conn) Serve(ctx context.Context) error {
	if c.OnOpen != nil {
		c.OnOpen(c.ID)
	}

	maxGear := c.effectiveMaxGear()
	minIntervalNs := c.effectiveMinIntervalNs()
	invalidThreshold := c.effectiveInvalidThreshold()
	invalidWindow := c.effectiveInvalidWindow()

	// Sentinel prev: seq=0 so the first real input (seq≥1) is always strictly
	// greater.
	var prev protocol.ClientInput
	prevWallNs := int64(0)

	// Rolling invalid-frame window.
	windowStart := time.Now()
	var windowCount uint32

	var loopErr error
	for {
		msgType, data, err := c.WS.Read(ctx)
		if err != nil {
			// Treat normal closure and io.EOF as clean exits.
			if errors.Is(err, io.EOF) ||
				websocket.CloseStatus(err) == websocket.StatusNormalClosure ||
				websocket.CloseStatus(err) == websocket.StatusGoingAway {
				loopErr = nil
			} else {
				loopErr = err
			}
			break
		}

		if msgType == websocket.MessageText {
			// Echo text frames (test compatibility; not part of the game protocol).
			if writeErr := c.WS.Write(ctx, websocket.MessageText, data); writeErr != nil {
				loopErr = writeErr
				break
			}
			continue
		}

		// Binary frame — attempt ClientInput decode.
		if len(data) < protocol.ClientInputSize {
			if kicked := c.handleInvalid(
				ctx, ReasonNone, errors.New("binary frame too short"),
				&windowStart, &windowCount, invalidThreshold, invalidWindow,
			); kicked {
				loopErr = errors.New("realtime: too many invalid frames")
				break
			}
			continue
		}

		var in protocol.ClientInput
		if umErr := in.Unmarshal(data); umErr != nil {
			if kicked := c.handleInvalid(
				ctx, ReasonNone, umErr,
				&windowStart, &windowCount, invalidThreshold, invalidWindow,
			); kicked {
				loopErr = errors.New("realtime: too many invalid frames")
				break
			}
			continue
		}

		nowNs := time.Now().UnixNano()
		reason, valErr := validateInput(
			&prev,
			prevWallNs,
			in,
			nowNs,
			maxGear,
			minIntervalNs,
		)
		if reason != ReasonNone {
			if kicked := c.handleInvalid(
				ctx, reason, valErr,
				&windowStart, &windowCount, invalidThreshold, invalidWindow,
			); kicked {
				loopErr = errors.New("realtime: too many invalid frames")
				break
			}
			continue
		}

		// Valid input: update prev state and enqueue.
		prev = in
		prevWallNs = nowNs
		c.ringMu.Lock()
		c.enqueue(in)
		c.ringMu.Unlock()
	}

	if c.OnClose != nil {
		c.OnClose(c.ID, loopErr)
	}
	return loopErr
}

// handleInvalid increments the invalid counter, calls InvalidReasonLog, and
// closes the connection if the rolling window threshold is breached.
// Returns true when the connection was kicked (caller should break the loop).
func (c *Conn) handleInvalid(
	ctx context.Context,
	reason InvalidReason,
	err error,
	windowStart *time.Time,
	windowCount *uint32,
	invalidThreshold uint32,
	invalidWindow time.Duration,
) bool {
	c.invalidCount.Add(1)

	if c.InvalidReasonLog != nil {
		c.InvalidReasonLog(c.ID, reason, err)
	}

	now := time.Now()
	if now.Sub(*windowStart) > invalidWindow {
		// Reset the rolling window.
		*windowStart = now
		*windowCount = 0
	}
	*windowCount++

	if *windowCount >= invalidThreshold {
		// CloseNow forcibly tears down the connection without the WS close
		// handshake so we don't block 5 s waiting for a client that is flooding
		// us.  WS.Read in the Serve loop will immediately return an error, but
		// since we return true the caller breaks before the next Read anyway.
		_ = c.WS.CloseNow()
		return true
	}
	return false
}

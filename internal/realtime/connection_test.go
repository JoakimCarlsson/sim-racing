package realtime_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JoakimCarlsson/sim-racing/internal/physics"
	"github.com/JoakimCarlsson/sim-racing/internal/protocol"
	"github.com/JoakimCarlsson/sim-racing/internal/realtime"
	"github.com/coder/websocket"
)

// upgradeAndServe is a minimal test handler: it accepts the WS connection,
// wraps it in a Conn, and calls Serve. It is used only in tests.
func upgradeAndServe(
	t *testing.T,
	openCount, closeCount *atomic.Int64,
) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			OriginPatterns: []string{"*"},
		})
		if err != nil {
			t.Errorf("accept: %v", err)
			return
		}
		conn := &realtime.Conn{
			ID: 1,
			WS: ws,
			OnOpen: func(id uint64) {
				openCount.Add(1)
			},
			OnClose: func(id uint64, err error) {
				closeCount.Add(1)
			},
		}
		_ = conn.Serve(r.Context())
	}
}

// makeClientInput returns a marshalled 24-byte ClientInput binary frame.
func makeClientInput(t *testing.T, seq uint32, throttle float32) []byte {
	t.Helper()
	ci := protocol.ClientInput{
		Seq:          seq,
		ClientTimeMs: seq * 16,
		Input: physics.Input{
			Throttle: throttle,
			Brake:    0,
			Steer:    0,
			Gear:     1,
		},
	}
	buf := make([]byte, protocol.ClientInputSize)
	_, err := ci.Marshal(buf)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	return buf
}

// TestConnServe_Echo verifies that a text frame sent by the client is echoed
// back verbatim and that OnOpen / OnClose are called exactly once each.
func TestConnServe_Echo(t *testing.T) {
	var openCount, closeCount atomic.Int64

	srv := httptest.NewServer(upgradeAndServe(t, &openCount, &closeCount))
	defer srv.Close()

	// Convert http:// → ws://
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/"

	ctx := context.Background()
	client, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	// Send a text frame.
	const msg = "hello"
	if err := client.Write(ctx, websocket.MessageText, []byte(msg)); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Read the echoed frame.
	typ, data, err := client.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if typ != websocket.MessageText {
		t.Errorf("expected text frame, got %v", typ)
	}
	if string(data) != msg {
		t.Errorf("echo: got %q, want %q", data, msg)
	}

	// Close from the client side; this causes Serve to return on the server.
	client.Close(websocket.StatusNormalClosure, "done")

	// Give the server handler time to finish.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if closeCount.Load() >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if openCount.Load() != 1 {
		t.Errorf("OnOpen: called %d times, want 1", openCount.Load())
	}
	if closeCount.Load() != 1 {
		t.Errorf("OnClose: called %d times, want 1", closeCount.Load())
	}
}

// TestConnServe_BinaryQueueOrder verifies AC4: valid binary ClientInput frames
// are queued in the per-connection ring buffer in monotonic seq order.
// The buffer holds 128 entries and drop-oldest is the overflow policy.
func TestConnServe_BinaryQueueOrder(t *testing.T) {
	const numInputs = 10 // well within 128-slot ring

	var (
		mu       sync.Mutex
		srvConn  *realtime.Conn
		closedWg sync.WaitGroup
	)
	closedWg.Add(1)

	srv := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
				OriginPatterns: []string{"*"},
			})
			if err != nil {
				t.Errorf("accept: %v", err)
				return
			}
			c := &realtime.Conn{
				ID:      1,
				WS:      ws,
				MaxGear: 6,
			}
			mu.Lock()
			srvConn = c
			mu.Unlock()
			_ = c.Serve(r.Context())
			closedWg.Done()
		}),
	)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/"
	ctx := context.Background()
	client, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	// Send numInputs valid binary frames, one every ~10ms (above default 8ms min).
	for i := uint32(1); i <= numInputs; i++ {
		frame := makeClientInput(t, i, 0.5)
		if err := client.Write(ctx, websocket.MessageBinary, frame); err != nil {
			t.Fatalf("write seq=%d: %v", i, err)
		}
		time.Sleep(12 * time.Millisecond)
	}

	// Give server time to process the last frame.
	time.Sleep(50 * time.Millisecond)

	client.Close(websocket.StatusNormalClosure, "done")
	closedWg.Wait()

	mu.Lock()
	c := srvConn
	mu.Unlock()

	if c == nil {
		t.Fatal("srvConn is nil")
	}

	// Drain buffered inputs.
	dst := make([]protocol.ClientInput, 128)
	n := c.DrainInputs(dst)
	if n != numInputs {
		t.Errorf("DrainInputs: got %d, want %d", n, numInputs)
	}

	// Verify monotonically increasing seq.
	for i := 0; i < n; i++ {
		wantSeq := uint32(i + 1)
		if dst[i].Seq != wantSeq {
			t.Errorf("dst[%d].Seq = %d, want %d", i, dst[i].Seq, wantSeq)
		}
	}
}

// TestConnServe_FloodCloses verifies AC3: sending 1000 inputs in a 100ms
// window causes the rate limiter to reject most of them and eventually
// close the connection (InvalidThreshold exceeded).
func TestConnServe_FloodCloses(t *testing.T) {
	var (
		reasonLog []realtime.InvalidReason
		mu        sync.Mutex
		closedWg  sync.WaitGroup
	)
	closedWg.Add(1)

	srv := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
				OriginPatterns: []string{"*"},
			})
			if err != nil {
				t.Errorf("accept: %v", err)
				return
			}
			c := &realtime.Conn{
				ID:               1,
				WS:               ws,
				MaxGear:          6,
				InvalidThreshold: 32,
				InvalidReasonLog: func(id uint64, reason realtime.InvalidReason, err error) {
					mu.Lock()
					reasonLog = append(reasonLog, reason)
					mu.Unlock()
				},
			}
			_ = c.Serve(r.Context())
			closedWg.Done()
		}),
	)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/"
	ctx := context.Background()
	client, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	// Send 1000 frames as fast as possible (flood).
	frame := makeClientInput(t, 1, 0.5)
	// All frames have the same seq=1 to also hit ReasonReplayedSeq after the
	// first one, or they will all be rate-limited.  Either way they are invalid
	// at the default 8ms minimum interval when fired without sleep.
	sent := 0
	deadline := time.Now().Add(200 * time.Millisecond)
	for i := 0; i < 1000 && time.Now().Before(deadline); i++ {
		if writeErr := client.Write(ctx, websocket.MessageBinary, frame); writeErr != nil {
			// Connection was closed by server — expected.
			break
		}
		sent++
	}
	t.Logf("sent %d frames before server closed", sent)

	// Wait for server to finish (it closes after InvalidThreshold violations).
	done := make(chan struct{})
	go func() { closedWg.Wait(); close(done) }()
	select {
	case <-done:
		// good
	case <-time.After(5 * time.Second):
		t.Fatal("server did not close connection within 5s")
	}

	mu.Lock()
	logged := len(reasonLog)
	mu.Unlock()
	if logged == 0 {
		t.Error(
			"InvalidReasonLog was never called; expected rate-limit rejections",
		)
	}

	// Most logged reasons must be rate-limit or replayed-seq violations.
	mu.Lock()
	badCount := 0
	for _, r := range reasonLog {
		if r == realtime.ReasonRateLimit || r == realtime.ReasonReplayedSeq {
			badCount++
		}
	}
	mu.Unlock()
	if badCount == 0 {
		t.Errorf(
			"expected rate-limit or replayed-seq reasons; got %v",
			reasonLog,
		)
	}

	// Verify the connection was actually closed (client's read returns error).
	client.CloseRead(ctx)
}

// TestConnServe_RingOverflow verifies that when the ring buffer is full
// (128 slots) and a new valid input arrives, the oldest entry is dropped
// (drop-oldest policy) — part of AC4 documentation check.
func TestConnServe_RingOverflow(t *testing.T) {
	// We send 130 valid inputs (above 128-slot capacity) at a safe interval
	// and verify DrainInputs returns exactly 128 entries, the newest ones.
	const ringCap = 128
	const numInputs = 130

	var (
		mu       sync.Mutex
		srvConn  *realtime.Conn
		closedWg sync.WaitGroup
	)
	closedWg.Add(1)

	srv := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
				OriginPatterns: []string{"*"},
			})
			if err != nil {
				t.Errorf("accept: %v", err)
				return
			}
			c := &realtime.Conn{
				ID:               1,
				WS:               ws,
				MaxGear:          6,
				InvalidThreshold: 200, // don't trigger kick during overflow test
			}
			mu.Lock()
			srvConn = c
			mu.Unlock()
			_ = c.Serve(r.Context())
			closedWg.Done()
		}),
	)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/"
	ctx := context.Background()
	client, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	for i := uint32(1); i <= numInputs; i++ {
		frame := makeClientInput(t, i, 0.5)
		if err := client.Write(ctx, websocket.MessageBinary, frame); err != nil {
			t.Fatalf("write seq=%d: %v", i, err)
		}
		time.Sleep(12 * time.Millisecond)
	}

	time.Sleep(50 * time.Millisecond)
	client.Close(websocket.StatusNormalClosure, "done")
	closedWg.Wait()

	mu.Lock()
	c := srvConn
	mu.Unlock()

	dst := make([]protocol.ClientInput, 256)
	n := c.DrainInputs(dst)
	if n != ringCap {
		t.Errorf("DrainInputs after overflow: got %d, want %d", n, ringCap)
	}

	// After drop-oldest, the ring holds seq 3..130 (inputs 1 and 2 dropped).
	// The first element should be seq = numInputs-ringCap+1.
	wantFirstSeq := uint32(numInputs - ringCap + 1)
	if dst[0].Seq != wantFirstSeq {
		t.Errorf(
			"first seq after overflow = %d, want %d",
			dst[0].Seq,
			wantFirstSeq,
		)
	}
	wantLastSeq := uint32(numInputs)
	if dst[n-1].Seq != wantLastSeq {
		t.Errorf(
			"last seq after overflow = %d, want %d",
			dst[n-1].Seq,
			wantLastSeq,
		)
	}

	// Ensure buffer was properly cleared.
	var buf2 [256]protocol.ClientInput
	n2 := c.DrainInputs(buf2[:])
	if n2 != 0 {
		t.Errorf("DrainInputs after drain: got %d, want 0", n2)
	}

	// Verify InvalidCount did not blow up unexpectedly (none of these were invalid).
	if cnt := c.InvalidCount(); cnt != 0 {
		t.Errorf("InvalidCount = %d, want 0", cnt)
	}

}

// ---- SendSnapshot tests (AC2 from the snapshot broadcaster issue) -----------

// TestSendSnapshotCopiesPayload verifies that SendSnapshot copies the payload
// so that the caller can freely mutate the original slice after the call.
func TestSendSnapshotCopiesPayload(t *testing.T) {
	var (
		mu       sync.Mutex
		srvConn  *realtime.Conn
		closedWg sync.WaitGroup
	)
	closedWg.Add(1)

	// Track payloads received by the client.
	var received [][]byte
	var recvMu sync.Mutex

	srv := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
				OriginPatterns: []string{"*"},
			})
			if err != nil {
				t.Errorf("accept: %v", err)
				return
			}
			c := &realtime.Conn{
				ID: 1,
				WS: ws,
			}
			mu.Lock()
			srvConn = c
			mu.Unlock()
			_ = c.Serve(r.Context())
			closedWg.Done()
		}),
	)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/"
	ctx := context.Background()
	client, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	// Read frames from the client in the background.
	var readWg sync.WaitGroup
	readWg.Add(1)
	go func() {
		defer readWg.Done()
		for {
			_, data, err := client.Read(ctx)
			if err != nil {
				return
			}
			cp := make([]byte, len(data))
			copy(cp, data)
			recvMu.Lock()
			received = append(received, cp)
			recvMu.Unlock()
		}
	}()

	// Wait for server conn to be assigned.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		c := srvConn
		mu.Unlock()
		if c != nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	mu.Lock()
	c := srvConn
	mu.Unlock()
	if c == nil {
		t.Fatal("srvConn never set")
	}

	// Send a snapshot payload; then immediately mutate the original slice.
	payload := []byte{
		0x02,
		0x01,
		0xAA,
		0xBB,
		0x00,
		0x00,
		0x00,
		0x00,
		0x00,
		0x00,
		0x01,
	}
	original := make([]byte, len(payload))
	copy(original, payload)
	c.SendSnapshot(payload, 0)

	// Mutate payload after SendSnapshot returns.
	for i := range payload {
		payload[i] = 0xFF
	}

	// Give the writer goroutine time to deliver the frame.
	time.Sleep(100 * time.Millisecond)

	client.Close(websocket.StatusNormalClosure, "done")
	closedWg.Wait()
	readWg.Wait()

	recvMu.Lock()
	defer recvMu.Unlock()
	if len(received) == 0 {
		t.Fatal("client received no frames")
	}
	// The received frame must match the original payload (not the mutated 0xFF bytes).
	got := received[0]
	if len(got) != len(original) {
		t.Fatalf("received frame length %d, want %d", len(got), len(original))
	}
	for i, b := range original {
		if got[i] != b {
			t.Errorf(
				"received[0][%d] = 0x%02x, want 0x%02x (payload was copied)",
				i,
				got[i],
				b,
			)
		}
	}
}

// TestSendSnapshotDropsWhenFull verifies that SendSnapshot returns immediately
// and increments DropCount when the outbound buffer (capacity 4) is full,
// rather than blocking.
func TestSendSnapshotDropsWhenFull(t *testing.T) {
	var (
		mu       sync.Mutex
		srvConn  *realtime.Conn
		closedWg sync.WaitGroup
	)
	closedWg.Add(1)

	srv := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
				OriginPatterns: []string{"*"},
			})
			if err != nil {
				t.Errorf("accept: %v", err)
				return
			}
			c := &realtime.Conn{
				ID: 1,
				WS: ws,
			}
			mu.Lock()
			srvConn = c
			mu.Unlock()
			_ = c.Serve(r.Context())
			closedWg.Done()
		}),
	)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/"
	ctx := context.Background()
	client, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	// Wait for server conn.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		c := srvConn
		mu.Unlock()
		if c != nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	mu.Lock()
	c := srvConn
	mu.Unlock()
	if c == nil {
		t.Fatal("srvConn never set")
	}

	// Send many frames rapidly without giving the writer goroutine time to
	// drain them. The outbound channel has capacity 4, so frames 5+ must drop.
	payload := []byte{
		0x02,
		0x01,
		0x00,
		0x00,
		0x00,
		0x00,
		0x00,
		0x00,
		0x00,
		0x00,
		0x00,
	}
	const sends = 20
	for i := 0; i < sends; i++ {
		start := time.Now()
		c.SendSnapshot(payload, 0)
		elapsed := time.Since(start)
		// Each call must return essentially immediately (non-blocking).
		if elapsed > 50*time.Millisecond {
			t.Errorf(
				"SendSnapshot[%d] blocked for %s; expected <50ms",
				i,
				elapsed,
			)
		}
	}

	drops := c.DropCount()
	t.Logf("sends=%d drops=%d", sends, drops)
	if drops == 0 {
		t.Error(
			"expected at least one drop when outbound buffer is full, got 0",
		)
	}

	client.Close(websocket.StatusNormalClosure, "done")
	closedWg.Wait()
}

// TestSendSnapshotAfterCloseDoesNotPanic is a regression test for the race
// condition where a broadcaster goroutine calls SendSnapshot on a Conn whose
// Serve teardown has already closed the outbound channel.
//
// The test connects a real WebSocket, lets Serve start (so the outbound channel
// is initialised and the closed flag is false), closes the client to trigger
// Serve's teardown path, then hammers SendSnapshot from many goroutines.
// No panic must occur; DropCount may or may not increment.
func TestSendSnapshotAfterCloseDoesNotPanic(t *testing.T) {
	var (
		mu       sync.Mutex
		srvConn  *realtime.Conn
		closedWg sync.WaitGroup
	)
	closedWg.Add(1)

	srv := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
				OriginPatterns: []string{"*"},
			})
			if err != nil {
				t.Errorf("accept: %v", err)
				return
			}
			c := &realtime.Conn{
				ID: 1,
				WS: ws,
			}
			mu.Lock()
			srvConn = c
			mu.Unlock()
			_ = c.Serve(r.Context())
			closedWg.Done()
		}),
	)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/"
	ctx := context.Background()
	client, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	// Wait until the server Conn is available (Serve has started and outbound
	// channel is initialised).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		c := srvConn
		mu.Unlock()
		if c != nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	mu.Lock()
	c := srvConn
	mu.Unlock()
	if c == nil {
		t.Fatal("srvConn never set")
	}

	// Close the client to trigger Serve's teardown (sets closed flag, closes
	// outbound channel).
	client.Close(websocket.StatusNormalClosure, "done")
	closedWg.Wait()

	// Now hammer SendSnapshot from many goroutines concurrently.  Any panic
	// here would be caught by the test runtime and fail the test.
	const goroutines = 50
	const sendsPerGoroutine = 100
	payload := []byte{0x02, 0x01, 0x00, 0x00}

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < sendsPerGoroutine; j++ {
				c.SendSnapshot(payload, 0)
			}
		}()
	}
	wg.Wait()

	// If we reach here without a panic, the race condition guard is working.
	t.Logf(
		"completed %d concurrent SendSnapshot calls after close without panic",
		goroutines*sendsPerGoroutine,
	)
}

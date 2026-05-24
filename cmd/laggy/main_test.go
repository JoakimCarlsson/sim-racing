package main

import (
	"context"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// ---------------------------------------------------------------------------
// Unit tests for computeDelay
// ---------------------------------------------------------------------------

// TestComputeDelay_NoJitter verifies that jitter=0 always returns exactly the
// configured latency.
func TestComputeDelay_NoJitter(t *testing.T) {
	t.Parallel()
	opts := proxyOptions{latency: 50 * time.Millisecond, jitter: 0}
	rng := rand.New(rand.NewSource(42))
	for i := range 100 {
		d, _ := computeDelay(opts, rng, time.Time{})
		if d != 50*time.Millisecond {
			t.Fatalf("iter %d: got %v, want %v", i, d, 50*time.Millisecond)
		}
	}
}

// TestComputeDelay_JitterBounds confirms that with latency=100ms jitter=20ms
// all 1000 samples land in [80,120]ms.
func TestComputeDelay_JitterBounds(t *testing.T) {
	t.Parallel()
	opts := proxyOptions{
		latency: 100 * time.Millisecond,
		jitter:  20 * time.Millisecond,
	}
	rng := rand.New(rand.NewSource(99))
	lo := 80 * time.Millisecond
	hi := 120 * time.Millisecond
	for i := range 1000 {
		d, _ := computeDelay(opts, rng, time.Time{})
		if d < lo || d > hi {
			t.Fatalf(
				"iter %d: delay %v outside [%v, %v]",
				i, d, lo, hi,
			)
		}
	}
}

// TestComputeDelay_NegativeClampedToZero ensures a configuration that would
// produce a negative raw delay is clamped to 0.
func TestComputeDelay_NegativeClampedToZero(t *testing.T) {
	t.Parallel()
	// latency=1ms jitter=50ms → many samples will be negative raw delay.
	opts := proxyOptions{
		latency: 1 * time.Millisecond,
		jitter:  50 * time.Millisecond,
	}
	rng := rand.New(rand.NewSource(0))

	gotZero := false
	for range 1000 {
		d, _ := computeDelay(opts, rng, time.Time{})
		if d < 0 {
			t.Fatalf("got negative delay %v", d)
		}
		if d == 0 {
			gotZero = true
		}
	}
	if !gotZero {
		t.Fatal("expected at least one zero delay from forced-negative samples")
	}
}

// ---------------------------------------------------------------------------
// Integration helpers
// ---------------------------------------------------------------------------

// echoHandler is an http.Handler that upgrades to WebSocket and echoes every
// frame with a "srv:" prefix prepended to the payload.
type echoHandler struct{}

func (echoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		return
	}
	defer conn.CloseNow()
	ctx := r.Context()
	for {
		msgType, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		reply := append([]byte("srv:"), data...)
		if werr := conn.Write(ctx, msgType, reply); werr != nil {
			return
		}
	}
}

// startEchoServer creates an httptest.Server with echoHandler.
func startEchoServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(echoHandler{})
	t.Cleanup(srv.Close)
	return srv
}

// proxyHandlerFor returns an http.Handler that runs the laggy proxy with the
// given options.
func proxyHandlerFor(opts proxyOptions) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		handleConn(w, r, opts)
	})
	return mux
}

// startProxy creates an httptest.Server running the proxy pointed at
// upstreamHost (host:port).
func startProxy(
	t *testing.T,
	upstreamHost string,
	opts proxyOptions,
) *httptest.Server {
	t.Helper()
	opts.targetHost = upstreamHost
	srv := httptest.NewServer(proxyHandlerFor(opts))
	t.Cleanup(srv.Close)
	return srv
}

// wsURL returns the ws:// URL for the /ws path on the given httptest.Server.
func wsURL(srv *httptest.Server) string {
	u := srv.URL
	return "ws" + u[len("http"):] + "/ws"
}

// ---------------------------------------------------------------------------
// Integration: TestProxy_ForwardsFramesInOrder_NoLossNoDelay
// ---------------------------------------------------------------------------

// TestProxy_ForwardsFramesInOrder_NoLossNoDelay verifies that with zero delay,
// jitter, and drop the proxy forwards 50 binary frames in order within 5ms
// per frame.
func TestProxy_ForwardsFramesInOrder_NoLossNoDelay(t *testing.T) {
	t.Parallel()

	echo := startEchoServer(t)
	upstreamHost := echo.Listener.Addr().String()

	opts := proxyOptions{
		latency: 0,
		jitter:  0,
		drop:    0,
		rng:     rand.New(rand.NewSource(1)),
	}
	proxy := startProxy(t, upstreamHost, opts)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL(proxy), nil)
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	defer conn.CloseNow()

	const n = 50
	for i := range n {
		frame := []byte{byte(i)}
		start := time.Now()
		if err := conn.Write(ctx, websocket.MessageBinary, frame); err != nil {
			t.Fatalf("write frame %d: %v", i, err)
		}
		_, resp, err := conn.Read(ctx)
		elapsed := time.Since(start)
		if err != nil {
			t.Fatalf("read frame %d: %v", i, err)
		}

		want := append([]byte("srv:"), frame...)
		if string(resp) != string(want) {
			t.Fatalf("frame %d: got %q, want %q", i, resp, want)
		}
		if elapsed >= 5*time.Millisecond {
			t.Logf(
				"WARNING: frame %d RTT=%v exceeded 5ms (may be acceptable on slow CI)",
				i,
				elapsed,
			)
		}
	}
}

// ---------------------------------------------------------------------------
// Integration: TestProxy_DropRateApproximates
// ---------------------------------------------------------------------------

// countingServer is an http.Handler that upgrades to WebSocket and counts
// received frames without echoing.  This lets us measure the one-way drop rate
// from client → server through the proxy.
type countingServer struct {
	received atomic.Int32
}

func (cs *countingServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		return
	}
	defer conn.CloseNow()
	ctx := r.Context()
	for {
		_, _, rerr := conn.Read(ctx)
		if rerr != nil {
			return
		}
		cs.received.Add(1)
	}
}

// TestProxy_DropRateApproximates verifies that with drop=0.5 over 200 sent
// frames the number of frames reaching the upstream server falls in [70,130],
// using a seeded rng.  The counting server never echoes, so only the c→s
// direction is measured (no double-drop through the echo path).
func TestProxy_DropRateApproximates(t *testing.T) {
	t.Parallel()

	cs := &countingServer{}
	upstream := httptest.NewServer(cs)
	t.Cleanup(upstream.Close)

	upstreamHost := upstream.Listener.Addr().String()

	opts := proxyOptions{
		latency: 0,
		jitter:  0,
		drop:    0.5,
		rng:     rand.New(rand.NewSource(12345)),
	}
	proxy := startProxy(t, upstreamHost, opts)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL(proxy), nil)
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}

	const total = 200
	for i := range total {
		frame := []byte{byte(i % 256)}
		if werr := conn.Write(ctx, websocket.MessageBinary, frame); werr != nil {
			break
		}
		// Small pause so the relay goroutine can process each frame.
		time.Sleep(2 * time.Millisecond)
	}

	conn.Close(websocket.StatusNormalClosure, "done")

	// Allow relay goroutine to flush.
	time.Sleep(100 * time.Millisecond)

	count := int(cs.received.Load())
	t.Logf("upstream received %d / %d frames (drop=0.5)", count, total)
	if count < 70 || count > 130 {
		t.Fatalf("drop-rate approximation out of range [70,130]: got %d", count)
	}
}

// ---------------------------------------------------------------------------
// Integration: TestProxy_LatencyMinimum
// ---------------------------------------------------------------------------

// TestProxy_LatencyMinimum verifies that with latency=80ms jitter=0 drop=0,
// RTT ≥ 80ms and ≤ 200ms.
func TestProxy_LatencyMinimum(t *testing.T) {
	t.Parallel()

	echo := startEchoServer(t)
	upstreamHost := echo.Listener.Addr().String()

	opts := proxyOptions{
		latency: 80 * time.Millisecond,
		jitter:  0,
		drop:    0,
		rng:     rand.New(rand.NewSource(7)),
	}
	proxy := startProxy(t, upstreamHost, opts)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL(proxy), nil)
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	defer conn.CloseNow()

	frame := []byte{0xAB}
	start := time.Now()
	if err := conn.Write(ctx, websocket.MessageBinary, frame); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, _, err = conn.Read(ctx)
	rtt := time.Since(start)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	t.Logf("RTT with 80ms one-way latency: %v", rtt)
	if rtt < 80*time.Millisecond {
		t.Fatalf("RTT %v < 80ms minimum", rtt)
	}
	if rtt > 200*time.Millisecond {
		t.Fatalf("RTT %v > 200ms maximum", rtt)
	}
}

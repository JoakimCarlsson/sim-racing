// cmd/laggy — dev-only WebSocket latency/jitter/loss proxy.
//
// Security / anti-cheat note: this proxy is protocol-agnostic.  It reads
// raw WebSocket frames and forwards them opaque, never parsing application
// payloads, so it cannot introduce any anti-cheat surface.
//
// Usage:
//
//	go run ./cmd/laggy --listen :9090 --target localhost:8080 \
//	                   --latency 75ms --jitter 20ms --drop 0.02
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

// proxyOptions holds all tuneable parameters, including an optional seeded
// *rand.Rand for deterministic unit tests.
type proxyOptions struct {
	listenAddr string
	targetHost string
	latency    time.Duration
	jitter     time.Duration
	drop       float64
	rng        *rand.Rand // nil → use global rand
}

// computeDelay returns a non-negative delay given the options.  The delay is
// latency ± uniform jitter, clamped to 0.  The rng argument must not be nil
// (pass the options' rng or a local one).
func computeDelay(opts proxyOptions, rng *rand.Rand, prev time.Time) (
	time.Duration,
	time.Time,
) {
	jitterRange := opts.jitter * 2
	var jitterOffset time.Duration
	if jitterRange > 0 {
		// uniform in [-jitter, +jitter)
		jitterOffset = time.Duration(rng.Int63n(int64(jitterRange))) - opts.jitter
	}
	raw := opts.latency + jitterOffset
	if raw < 0 {
		raw = 0
	}

	// FIFO clamp: scheduled time must be >= previous scheduled time.
	now := time.Now()
	scheduled := now.Add(raw)
	if !prev.IsZero() && scheduled.Before(prev) {
		scheduled = prev
	}
	return scheduled.Sub(now), scheduled
}

// relayDirection copies frames from src → dst, applying latency, jitter and
// drop.  It signals done on the provided channel when it exits (error or EOF).
func relayDirection(
	ctx context.Context,
	src, dst *websocket.Conn,
	opts proxyOptions,
	rng *rand.Rand,
	dropped *atomic.Int64,
	label string,
	done chan<- struct{},
) {
	defer func() { done <- struct{}{} }()

	var prevScheduled time.Time

	for {
		msgType, data, err := src.Read(ctx)
		if err != nil {
			if ctx.Err() == nil {
				// Unexpected error — log at debug level only (EOF is normal).
				if err != io.EOF {
					log.Printf("[laggy] %s read error: %v", label, err)
				}
			}
			return
		}

		delay, scheduled := computeDelay(opts, rng, prevScheduled)
		prevScheduled = scheduled

		// Drop check — decide before sleeping so drops don't stall the queue.
		shouldDrop := opts.drop > 0 && rng.Float64() < opts.drop
		if shouldDrop {
			dropped.Add(1)
			log.Printf("[laggy] %s dropped frame (%d total)", label, dropped.Load())
			continue
		}

		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return
			}
		}

		if err := dst.Write(ctx, msgType, data); err != nil {
			if ctx.Err() == nil {
				log.Printf("[laggy] %s write error: %v", label, err)
			}
			return
		}
	}
}

// handleConn accepts a single browser WS connection, dials the upstream, and
// starts two relay goroutines (one per direction).
func handleConn(
	w http.ResponseWriter,
	r *http.Request,
	opts proxyOptions,
) {
	// Accept the browser side.
	clientConn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		// websocket.Accept already wrote an HTTP error response.
		log.Printf("[laggy] accept error: %v", err)
		return
	}

	// Dial the upstream (the real game server).
	targetURL := fmt.Sprintf("ws://%s/ws", opts.targetHost)
	serverConn, _, err := websocket.Dial(r.Context(), targetURL, nil)
	if err != nil {
		log.Printf("[laggy] dial upstream %s error: %v", targetURL, err)
		_ = clientConn.Close(websocket.StatusBadGateway, "upstream unavailable")
		return
	}

	log.Printf(
		"[laggy] accepted conn from %s → %s (latency=%v jitter=%v drop=%.2f)",
		r.RemoteAddr, targetURL, opts.latency, opts.jitter, opts.drop,
	)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Give each relay direction its own rng so they do not race on the same
	// source.  If opts.rng is set (deterministic test mode) we seed both from
	// it; otherwise we derive from wall time.
	var rngCS, rngSC *rand.Rand
	if opts.rng != nil {
		rngCS = rand.New(rand.NewSource(opts.rng.Int63()))
		rngSC = rand.New(rand.NewSource(opts.rng.Int63()))
	} else {
		now := time.Now().UnixNano()
		rngCS = rand.New(rand.NewSource(now))     //nolint:gosec
		rngSC = rand.New(rand.NewSource(now + 1)) //nolint:gosec
	}

	var dropped atomic.Int64
	done := make(chan struct{}, 2)

	go relayDirection(ctx, clientConn, serverConn, opts, rngCS, &dropped, "c→s", done)
	go relayDirection(ctx, serverConn, clientConn, opts, rngSC, &dropped, "s→c", done)

	// Wait for the first direction to finish; cancel context to stop the other.
	<-done
	cancel()
	<-done

	total := dropped.Load()
	log.Printf(
		"[laggy] closed conn from %s (dropped=%d)",
		r.RemoteAddr, total,
	)

	_ = clientConn.Close(websocket.StatusNormalClosure, "proxy closed")
	_ = serverConn.Close(websocket.StatusNormalClosure, "proxy closed")
}

func main() {
	var (
		listenAddr string
		targetHost string
		latency    time.Duration
		jitter     time.Duration
		drop       float64
	)

	flag.StringVar(&listenAddr, "listen", ":9090", "proxy listen address")
	flag.StringVar(&targetHost, "target", "localhost:8080", "upstream server host:port")
	flag.DurationVar(&latency, "latency", 75*time.Millisecond, "base one-way latency")
	flag.DurationVar(&jitter, "jitter", 20*time.Millisecond, "one-way jitter ±")
	flag.Float64Var(&drop, "drop", 0.0, "frame drop probability [0,1)")
	flag.Parse()

	opts := proxyOptions{
		listenAddr: listenAddr,
		targetHost: targetHost,
		latency:    latency,
		jitter:     jitter,
		drop:       drop,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		handleConn(w, r, opts)
	})

	log.Printf("[laggy] listening on %s (target=%s latency=%v jitter=%v drop=%.2f)",
		listenAddr, targetHost, latency, jitter, drop)

	if err := http.ListenAndServe(listenAddr, mux); err != nil { //nolint:gosec
		log.Fatalf("[laggy] server error: %v", err)
	}
}

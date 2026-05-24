package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	httpx "github.com/JoakimCarlsson/sim-racing/internal/http"
	"github.com/JoakimCarlsson/sim-racing/internal/protocol"
	"github.com/JoakimCarlsson/sim-racing/internal/sim"
	"github.com/coder/websocket"
)

func TestSnapshotBroadcastSmoke(t *testing.T) {
	world := sim.New()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go world.Run(ctx)
	go world.RunBroadcaster(ctx, 30, time.Now)

	handler := httpx.NewHandler(httpx.Deps{
		StartedAt: time.Now(),
		Version:   "smoke",
		World:     world,
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"

	wsCtx, wsCancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer wsCancel()

	conn, _, err := websocket.Dial(wsCtx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{"http://localhost:5173"}},
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.CloseNow()

	frameCount := 0
	start := time.Now()
	deadline := start.Add(2 * time.Second)
	for time.Now().Before(deadline) {
		readCtx, readCancel := context.WithDeadline(wsCtx, deadline)
		_, data, readErr := conn.Read(readCtx)
		readCancel()
		if readErr != nil {
			break
		}
		frameCount++
		if frameCount == 1 {
			if len(data) < 2 {
				t.Errorf("first frame too short: %d bytes", len(data))
				break
			}
			if data[0] != byte(protocol.MsgServerSnapshot) {
				t.Errorf(
					"first frame byte 0 = 0x%02x, want 0x%02x (MsgServerSnapshot)",
					data[0],
					byte(protocol.MsgServerSnapshot),
				)
			}
			if data[1] != protocol.ProtocolVersion {
				t.Errorf(
					"first frame byte 1 = %d, want %d (ProtocolVersion)",
					data[1],
					protocol.ProtocolVersion,
				)
			}
			t.Logf(
				"first frame: len=%d bytes[0:2]=[%02x %02x]",
				len(data),
				data[0],
				data[1],
			)
		}
	}
	elapsed := time.Since(start)
	hz := float64(frameCount) / elapsed.Seconds()
	t.Logf(
		"received %d frames in %v = %.1f Hz",
		frameCount,
		elapsed.Round(time.Millisecond),
		hz,
	)

	const tolerance = 0.05
	expectedFrames := 30.0 * elapsed.Seconds()
	ratio := float64(frameCount) / expectedFrames
	if ratio < 1-tolerance || ratio > 1+tolerance {
		t.Errorf(
			"snapshot Hz out of tolerance: %.1f Hz (ratio=%.3f)",
			hz,
			ratio,
		)
	}

	conn.Close(websocket.StatusNormalClosure, "done")
}

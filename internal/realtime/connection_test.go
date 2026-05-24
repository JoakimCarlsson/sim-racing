package realtime_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

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

// TestConnServe_Echo verifies that a text frame sent by the client is echoed
// back verbatim (AC2) and that OnOpen / OnClose are called exactly once each.
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
	// We poll briefly to avoid a hard sleep.
	for i := 0; i < 50; i++ {
		if closeCount.Load() >= 1 {
			break
		}
	}

	if openCount.Load() != 1 {
		t.Errorf("OnOpen: called %d times, want 1", openCount.Load())
	}
	if closeCount.Load() != 1 {
		t.Errorf("OnClose: called %d times, want 1", closeCount.Load())
	}
}

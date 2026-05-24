// Package realtime owns WebSocket connection lifecycle for sim-racing clients.
package realtime

import (
	"context"
	"errors"
	"io"

	"github.com/coder/websocket"
)

// Conn wraps a single WebSocket connection and its lifecycle callbacks.
type Conn struct {
	// ID is a monotonically increasing identifier assigned by the HTTP layer.
	ID uint64

	// WS is the accepted WebSocket connection.
	WS *websocket.Conn

	// OnOpen is called once immediately before the read loop starts.
	// May be nil.
	OnOpen func(id uint64)

	// OnClose is called once after the read loop exits, with the error that
	// caused it to exit (nil on a clean close).
	// May be nil.
	OnClose func(id uint64, err error)
}

// Serve runs the connection's read loop: it calls OnOpen, reads frames, echoes
// text frames verbatim, ignores binary frames, and calls OnClose on exit.
// It blocks until the connection is closed or ctx is cancelled.
func (c *Conn) Serve(ctx context.Context) error {
	if c.OnOpen != nil {
		c.OnOpen(c.ID)
	}

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

		if msgType != websocket.MessageText {
			// Binary frames are ignored per plan non-goals.
			continue
		}

		if err := c.WS.Write(ctx, websocket.MessageText, data); err != nil {
			loopErr = err
			break
		}
	}

	if c.OnClose != nil {
		c.OnClose(c.ID, loopErr)
	}
	return loopErr
}

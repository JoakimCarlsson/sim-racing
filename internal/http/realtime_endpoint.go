package httpx

import (
	"log"
	"sync/atomic"

	"github.com/JoakimCarlsson/sim-racing/internal/realtime"
	"github.com/coder/websocket"
	"github.com/joakimcarlsson/minmux/router"
)

var connIDCounter atomic.Uint64

func registerRealtime(r *router.Router, _ Deps) {
	r.Get("/ws", func(c *router.Context) {
		ws, err := websocket.Accept(c.Writer, c.Request, &websocket.AcceptOptions{
			// Allow connections from localhost on any port (e.g. Vite dev server on 5173).
			OriginPatterns: []string{"localhost:*"},
		})
		if err != nil {
			// websocket.Accept has already written the error response (400/403).
			return
		}

		id := connIDCounter.Add(1)
		conn := &realtime.Conn{
			ID: id,
			WS: ws,
			OnOpen: func(id uint64) {
				log.Printf("ws open id=%d remote=%s", id, c.Request.RemoteAddr)
			},
			OnClose: func(id uint64, err error) {
				if err != nil {
					log.Printf("ws close id=%d err=%v", id, err)
				} else {
					log.Printf("ws close id=%d err=<nil>", id)
				}
			},
		}

		_ = conn.Serve(c.Request.Context())
	})
}

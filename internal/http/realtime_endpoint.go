package httpx

import (
	"log"
	"sync/atomic"

	"github.com/JoakimCarlsson/sim-racing/internal/physics"
	"github.com/JoakimCarlsson/sim-racing/internal/realtime"
	"github.com/JoakimCarlsson/sim-racing/internal/sim"
	"github.com/coder/websocket"
	"github.com/joakimcarlsson/minmux/router"
)

var connIDCounter atomic.Uint64

func registerRealtime(r *router.Router, deps Deps) {
	r.Get("/ws", func(c *router.Context) {
		ws, err := websocket.Accept(
			c.Writer,
			c.Request,
			&websocket.AcceptOptions{
				// Allow connections from localhost on any port (e.g. Vite dev server on 5173).
				OriginPatterns: []string{"localhost:*"},
			},
		)
		if err != nil {
			// websocket.Accept writes 426 Upgrade Required when the request is
			// not a valid WebSocket upgrade (LEARNINGS #54).
			return
		}

		rawID := connIDCounter.Add(1)
		// TODO: use a proper PlayerID allocator; for now cast uint64 → uint16.
		playerID := sim.PlayerID(rawID)

		// conn is declared before the closures reference it so that OnOpen can
		// pass conn as the InputSource.
		conn := &realtime.Conn{
			ID: rawID,
			WS: ws,
		}

		conn.InvalidReasonLog = func(id uint64, reason realtime.InvalidReason, err error) {
			log.Printf("ws invalid id=%d reason=%s err=%v", id, reason, err)
		}
		conn.OnOpen = func(id uint64) {
			log.Printf("ws open id=%d remote=%s", id, c.Request.RemoteAddr)
			if deps.World != nil {
				p := &sim.PlayerSim{
					ID:     playerID,
					Source: conn,
					Sink:   conn,
					State: physics.State{
						// Identity quaternion: x=0 y=0 z=0 w=1
						Orientation: [4]float32{0, 0, 0, 1},
					},
					Constants: physics.DefaultConstants,
				}
				deps.World.AddPlayer(p)
			}
		}
		conn.OnClose = func(id uint64, err error) {
			if err != nil {
				log.Printf("ws close id=%d err=%v", id, err)
			} else {
				log.Printf("ws close id=%d err=<nil>", id)
			}
			if deps.World != nil {
				deps.World.RemovePlayer(playerID)
			}
		}

		_ = conn.Serve(c.Request.Context())
	})
}

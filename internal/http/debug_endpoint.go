package httpx

import (
	"net/http"

	"github.com/joakimcarlsson/minmux/router"
)

// playerDebugDTO is the JSON shape for a single player in GET /debug/players.
type playerDebugDTO struct {
	PlayerID  uint16  `json:"playerID"`
	WheelsOff uint8   `json:"wheelsOff"`
	PosX      float32 `json:"posX"`
	PosY      float32 `json:"posY"`
	PosZ      float32 `json:"posZ"`
}

func registerDebug(r *router.Router, deps Deps) {
	r.Get("/debug/players", func(c *router.Context) {
		if deps.World == nil {
			c.JSON(http.StatusOK, []playerDebugDTO{})
			return
		}

		snapshots := deps.World.Snapshots()
		out := make([]playerDebugDTO, 0, len(snapshots))
		for _, s := range snapshots {
			out = append(out, playerDebugDTO{
				PlayerID:  s.ID,
				WheelsOff: s.WheelsOff,
				PosX:      s.State.Position[0],
				PosY:      s.State.Position[1],
				PosZ:      s.State.Position[2],
			})
		}
		c.JSON(http.StatusOK, out)
	})
}

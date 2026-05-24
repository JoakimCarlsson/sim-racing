package httpx

import (
	"net/http"

	"github.com/JoakimCarlsson/sim-racing/internal/health"
	"github.com/joakimcarlsson/minmux/router"
)

// healthResponse is the JSON shape for GET /healthz.
type healthResponse struct {
	Status        string `json:"status"`
	Version       string `json:"version"`
	UptimeSeconds int64  `json:"uptime_seconds"`
}

func registerHealth(r *router.Router, deps Deps) {
	svc := health.New(deps.StartedAt, deps.Version)

	r.Get("/healthz", func(c *router.Context) {
		res := svc.Status()
		c.JSON(http.StatusOK, healthResponse{
			Status:        "ok",
			Version:       res.Version,
			UptimeSeconds: res.UptimeSeconds,
		})
	})
}

// Package httpx assembles the minmux router and registers all subsystem routes.
package httpx

import (
	"net/http"
	"time"

	"github.com/JoakimCarlsson/sim-racing/internal/sim"
	"github.com/joakimcarlsson/minmux/router"
)

// Deps holds the dependencies injected into the HTTP handler at startup.
type Deps struct {
	StartedAt time.Time
	Version   string
	World     *sim.World
}

// NewHandler constructs the root http.Handler with all routes registered.
func NewHandler(deps Deps) http.Handler {
	r := router.New()
	registerHealth(r, deps)
	registerRealtime(r, deps)
	return r
}

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	httpx "github.com/JoakimCarlsson/sim-racing/internal/http"
	"github.com/JoakimCarlsson/sim-racing/internal/physics"
	"github.com/JoakimCarlsson/sim-racing/internal/sim"
	"github.com/JoakimCarlsson/sim-racing/internal/track"
)

// Compile-time assertion: *track.Heightmap must satisfy physics.GroundSampler.
// This is guaranteed by Heightmap.Sample having the identical signature:
//
//	func (h *Heightmap) Sample(x, z float32) (height float32, normal [3]float32, ok bool)
var _ physics.GroundSampler = (*track.Heightmap)(nil)

// snapshotHz is the rate at which the broadcaster sends snapshots to clients.
const snapshotHz = 30

var version = "dev"

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	world := sim.New()

	// Load track metadata and optional heightmap.
	trackDir := os.Getenv("TRACK_DIR")
	if trackDir == "" {
		trackDir = filepath.Join(".", "assets", "tracks", "circuit01")
	}
	loadTrack(trackDir, world)

	// Run the authoritative tick loop until the process receives a termination
	// signal.
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	go world.Run(ctx)
	go world.RunBroadcaster(ctx, snapshotHz, time.Now)

	handler := httpx.NewHandler(httpx.Deps{
		StartedAt: time.Now(),
		Version:   version,
		World:     world,
	})

	log.Printf("sim-racing server starting on :%s (version=%s)", port, version)
	if err := http.ListenAndServe(":"+port, handler); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// loadTrack loads track.json and (optionally) track.height.bin from dir.
// When a heightmap is present it is wired into world.Ground so physics uses
// real terrain elevations. A missing .height.bin is a warning, not a fatal
// error — the world keeps its default FlatGround(0) sampler.
func loadTrack(dir string, world *sim.World) {
	jsonPath := filepath.Join(dir, "track.json")
	tr, err := track.LoadFile(jsonPath)
	if err != nil {
		log.Printf("track: load %s: %v", jsonPath, err)
		return
	}

	heightsDesc := "none"
	binPath := filepath.Join(dir, "track.height.bin")
	hm, err := track.LoadHeightmapFile(binPath)
	if err != nil {
		log.Printf("track: heightmap not loaded (continuing without): %v", err)
	} else {
		tr.Heights = hm
		heightsDesc = fmt.Sprintf(
			"%dx%d@%.4gm",
			hm.Width,
			hm.Depth,
			hm.CellSize,
		)
		// Wire the heightmap into the sim world as the active GroundSampler.
		world.Ground = hm
	}

	log.Printf(
		"track loaded: id=%s heights=%s",
		tr.ID,
		heightsDesc,
	)
}

package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	httpx "github.com/JoakimCarlsson/sim-racing/internal/http"
	"github.com/JoakimCarlsson/sim-racing/internal/sim"
)

var version = "dev"

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	world := sim.New()

	// Run the authoritative tick loop until the process receives a termination
	// signal.
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	go world.Run(ctx)

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

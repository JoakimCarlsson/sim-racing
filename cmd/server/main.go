package main

import (
	"log"
	"net/http"
	"os"
	"time"

	httpx "github.com/JoakimCarlsson/sim-racing/internal/http"
)

var version = "dev"

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	handler := httpx.NewHandler(httpx.Deps{
		StartedAt: time.Now(),
		Version:   version,
	})

	log.Printf("sim-racing server starting on :%s (version=%s)", port, version)
	if err := http.ListenAndServe(":"+port, handler); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/GOG-777/wrap-link/pkg/wrap"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dataFile := os.Getenv("WRAP_DATA_FILE")
	if dataFile == "" {
		dataFile = filepath.Join("data", "links.json")
	}

	srv := wrap.NewServer(dataFile, "public")

	httpServer := &http.Server{
		Addr:              ":" + port,
		Handler:           srv,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Printf("wrap-link listening on http://localhost:%s", port)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

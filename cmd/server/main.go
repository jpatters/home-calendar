package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jpatters/home-calendar/internal/config"
	"github.com/jpatters/home-calendar/internal/server"
)

func main() {
	configPath := envOr("CONFIG_PATH", "/data/config.json")
	addr := envOr("LISTEN_ADDR", ":8080")

	store, err := config.Open(configPath)
	if err != nil {
		log.Fatalf("open config: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	srv, handler, err := server.New(ctx, store)
	if err != nil {
		log.Fatalf("start server: %v", err)
	}
	defer srv.Shutdown()

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("home-calendar listening on %s (config: %s)", addr, configPath)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-ctx.Done()
	log.Print("shutting down")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = httpServer.Shutdown(shutdownCtx)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

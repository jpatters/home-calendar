package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/jpatters/home-calendar/internal/config"
	"github.com/jpatters/home-calendar/internal/secrets"
	"github.com/jpatters/home-calendar/internal/server"
)

func main() {
	configPath := envOr("CONFIG_PATH", "/data/config.json")
	addr := envOr("LISTEN_ADDR", ":8080")

	log.Printf("local time zone: %s (TZ=%q)", time.Now().Format("MST -07:00"), os.Getenv("TZ"))

	store, sec, err := openStores(configPath)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	srv, handler, err := server.New(ctx, server.Options{Config: store, Secrets: sec})
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

// openStores opens the config file and the secrets file beside it.
func openStores(configPath string) (*config.Store, *secrets.Store, error) {
	store, err := config.Open(configPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open config: %w", err)
	}
	sec, err := secrets.Open(secrets.PathFor(configPath))
	if err != nil {
		return nil, nil, fmt.Errorf("open secrets: %w", err)
	}
	return store, sec, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

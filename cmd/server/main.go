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
	"github.com/jpatters/home-calendar/internal/memories"
	"github.com/jpatters/home-calendar/internal/secrets"
	"github.com/jpatters/home-calendar/internal/server"
)

func main() {
	configPath := envOr("CONFIG_PATH", "/data/config.json")
	addr := envOr("LISTEN_ADDR", ":8080")

	st, err := openStores(configPath)
	if err != nil {
		log.Fatal(err)
	}
	store := st.config
	// Before anything else starts, since everything reads time.Local.
	if loc := zoneFallback(os.Getenv("TZ"), store.Get().Weather.Timezone); loc != nil {
		time.Local = loc
	}
	log.Printf("local time zone: %s %s (TZ=%q)", time.Local, time.Now().Format("MST -07:00"), os.Getenv("TZ"))

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	srv, handler, err := server.New(ctx, server.Options{Config: store, Secrets: st.secrets, Memories: st.memories})
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

// zoneFallback is the time zone to run in when TZ is empty (unset, or set
// empty by compose), which would otherwise mean UTC: the weather location's
// zone from config. It returns nil to keep the default, when TZ is set or the
// weather zone isn't a real one (such as "auto"). A changed weather location
// takes effect on the next restart.
func zoneFallback(tz, weatherTZ string) *time.Location {
	if tz != "" || weatherTZ == "" {
		return nil
	}
	loc, err := time.LoadLocation(weatherTZ)
	if err != nil || loc == time.UTC {
		return nil
	}
	return loc
}

type stores struct {
	config   *config.Store
	secrets  *secrets.Store
	memories *memories.Store
}

// openStores opens the config file and the secrets and memories files beside
// it.
func openStores(configPath string) (stores, error) {
	cfg, err := config.Open(configPath)
	if err != nil {
		return stores{}, fmt.Errorf("open config: %w", err)
	}
	sec, err := secrets.Open(secrets.PathFor(configPath))
	if err != nil {
		return stores{}, fmt.Errorf("open secrets: %w", err)
	}
	mem, err := memories.Open(memories.PathFor(configPath))
	if err != nil {
		return stores{}, fmt.Errorf("open memories: %w", err)
	}
	return stores{config: cfg, secrets: sec, memories: mem}, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

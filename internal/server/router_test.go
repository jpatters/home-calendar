package server

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/jpatters/home-calendar/internal/baseball"
	"github.com/jpatters/home-calendar/internal/config"
	"github.com/jpatters/home-calendar/internal/ical"
	"github.com/jpatters/home-calendar/internal/snowday"
	"github.com/jpatters/home-calendar/internal/tide"
	"github.com/jpatters/home-calendar/internal/types"
	"github.com/jpatters/home-calendar/internal/weather"
)

func TestRestartFetchersBroadcastsClearingFrameWhenWidgetsDisabled(t *testing.T) {
	dir := t.TempDir()
	store, err := config.Open(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatalf("config.Open: %v", err)
	}
	cfg := store.Get()
	cfg.Weather.Enabled = false
	cfg.Tide.Enabled = false
	cfg.SnowDay.Enabled = false
	cfg.Baseball.Enabled = false
	cfg.Display.CalendarEnabled = false
	if _, err := store.Replace(cfg); err != nil {
		t.Fatalf("Replace: %v", err)
	}

	hub := NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := &Server{cfg: store, hub: hub, rootCtx: ctx}
	srv.ical = ical.New(func(events []types.Event) {
		hub.Broadcast(Frame{Type: "calendar", Events: events})
	})
	srv.weather = weather.New(func(snap *types.WeatherSnapshot) {
		hub.Broadcast(Frame{Type: "weather", Weather: snap})
	})
	srv.snowday = snowday.New(func(snap *types.SnowDaySnapshot) {
		hub.Broadcast(Frame{Type: "snowday", SnowDay: snap})
	})
	srv.tide = tide.New(func(snap *types.TideSnapshot) {
		hub.Broadcast(Frame{Type: "tide", Tide: snap})
	})
	srv.baseball = baseball.New(func(snap *types.BaseballSnapshot) {
		hub.Broadcast(Frame{Type: "baseball", Baseball: snap})
	})

	client := hub.register()
	defer hub.unregister(client)

	srv.restartFetchers(cfg)

	got := collectFrameTypes(t, client, 5, 500*time.Millisecond)
	if !got["weather"] {
		t.Errorf("missing clearing frame for weather")
	}
	if !got["tide"] {
		t.Errorf("missing clearing frame for tide")
	}
	if !got["snowday"] {
		t.Errorf("missing clearing frame for snowday")
	}
	if !got["calendar"] {
		t.Errorf("missing clearing frame for calendar")
	}
	if !got["baseball"] {
		t.Errorf("missing clearing frame for baseball")
	}
}

func TestRestartFetchersClearingFrameCarriesNilSnapshot(t *testing.T) {
	dir := t.TempDir()
	store, err := config.Open(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatalf("config.Open: %v", err)
	}
	cfg := store.Get()
	cfg.Weather.Enabled = false
	cfg.Tide.Enabled = true
	cfg.SnowDay.Enabled = true
	cfg.Display.CalendarEnabled = true
	if _, err := store.Replace(cfg); err != nil {
		t.Fatalf("Replace: %v", err)
	}

	hub := NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := &Server{cfg: store, hub: hub, rootCtx: ctx}
	srv.ical = ical.New(func(events []types.Event) {
		hub.Broadcast(Frame{Type: "calendar", Events: events})
	})
	srv.weather = weather.New(func(snap *types.WeatherSnapshot) {
		hub.Broadcast(Frame{Type: "weather", Weather: snap})
	})
	srv.snowday = snowday.New(func(snap *types.SnowDaySnapshot) {
		hub.Broadcast(Frame{Type: "snowday", SnowDay: snap})
	})
	srv.tide = tide.New(func(snap *types.TideSnapshot) {
		hub.Broadcast(Frame{Type: "tide", Tide: snap})
	})
	srv.baseball = baseball.New(func(snap *types.BaseballSnapshot) {
		hub.Broadcast(Frame{Type: "baseball", Baseball: snap})
	})

	client := hub.register()
	defer hub.unregister(client)

	srv.restartFetchers(cfg)

	// Look for the weather clearing frame specifically and assert Weather is nil.
	deadline := time.After(500 * time.Millisecond)
	for {
		select {
		case data := <-client.send:
			var f Frame
			if err := json.Unmarshal(data, &f); err != nil {
				t.Fatalf("decode frame: %v", err)
			}
			if f.Type == "weather" {
				if f.Weather != nil {
					t.Errorf("weather clearing frame should carry nil snapshot, got %+v", f.Weather)
				}
				return
			}
		case <-deadline:
			t.Fatalf("never saw weather clearing frame")
		}
	}
}

func collectFrameTypes(t *testing.T, c *client, want int, timeout time.Duration) map[string]bool {
	t.Helper()
	got := map[string]bool{}
	deadline := time.After(timeout)
	for len(got) < want {
		select {
		case data := <-c.send:
			var f Frame
			if err := json.Unmarshal(data, &f); err != nil {
				t.Fatalf("decode frame: %v", err)
			}
			got[f.Type] = true
		case <-deadline:
			return got
		}
	}
	return got
}

package server

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/jpatters/home-calendar/internal/baseball"
	"github.com/jpatters/home-calendar/internal/config"
	"github.com/jpatters/home-calendar/internal/ical"
	"github.com/jpatters/home-calendar/internal/snowday"
	"github.com/jpatters/home-calendar/internal/tide"
	"github.com/jpatters/home-calendar/internal/types"
	"github.com/jpatters/home-calendar/internal/weather"
)

type Server struct {
	cfg        *config.Store
	ical       *ical.Fetcher
	weather    *weather.Fetcher
	snowday    *snowday.Fetcher
	tide       *tide.Fetcher
	baseball   *baseball.Fetcher
	hub        *Hub
	rootCtx    context.Context
	geocode    func(ctx context.Context, query string) ([]weather.GeoResult, error)
	teamSearch func(ctx context.Context, query string) ([]baseball.TeamResult, error)
}

func New(ctx context.Context, cfg *config.Store) (*Server, http.Handler, error) {
	hub := NewHub()
	s := &Server{
		cfg:     cfg,
		hub:     hub,
		rootCtx: ctx,
	}
	s.ical = ical.New(func(events []types.Event) {
		hub.Broadcast(Frame{Type: "calendar", Events: events})
	})
	s.weather = weather.New(func(snap *types.WeatherSnapshot) {
		hub.Broadcast(Frame{Type: "weather", Weather: snap})
	})
	s.snowday = snowday.New(func(snap *types.SnowDaySnapshot) {
		hub.Broadcast(Frame{Type: "snowday", SnowDay: snap})
	})
	s.tide = tide.New(func(snap *types.TideSnapshot) {
		hub.Broadcast(Frame{Type: "tide", Tide: snap})
	})
	s.baseball = baseball.New(func(snap *types.BaseballSnapshot) {
		hub.Broadcast(Frame{Type: "baseball", Baseball: snap})
	})
	s.geocode = func(ctx context.Context, q string) ([]weather.GeoResult, error) {
		return weather.Search(ctx, s.weather.HTTPClient(), weather.DefaultGeocodingURL, q)
	}
	s.teamSearch = func(ctx context.Context, q string) ([]baseball.TeamResult, error) {
		return baseball.SearchTeams(ctx, s.baseball.HTTPClient(), baseball.DefaultTeamsURL, q)
	}

	current := cfg.Get()
	s.applyFetcherConfig(ctx, current, false)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/config", s.handleGetConfig)
	mux.HandleFunc("PUT /api/config", s.handlePutConfig)
	mux.HandleFunc("GET /api/calendar/events", s.handleGetEvents)
	mux.HandleFunc("POST /api/calendar/refresh", s.handleCalendarRefresh)
	mux.HandleFunc("GET /api/weather", s.handleGetWeather)
	mux.HandleFunc("POST /api/weather/refresh", s.handleWeatherRefresh)
	mux.HandleFunc("GET /api/weather/geocode", s.handleWeatherGeocode)
	mux.HandleFunc("GET /api/snowday", s.handleGetSnowDay)
	mux.HandleFunc("POST /api/snowday/refresh", s.handleSnowDayRefresh)
	mux.HandleFunc("GET /api/tide", s.handleGetTide)
	mux.HandleFunc("POST /api/tide/refresh", s.handleTideRefresh)
	mux.HandleFunc("GET /api/baseball", s.handleGetBaseball)
	mux.HandleFunc("POST /api/baseball/refresh", s.handleBaseballRefresh)
	mux.HandleFunc("GET /api/baseball/teams", s.handleBaseballTeamSearch)
	mux.HandleFunc("GET /api/ws", s.handleWS)

	spa, err := newSPAHandler()
	if err != nil {
		return nil, nil, err
	}
	mux.Handle("/", spa)

	return s, logging(mux), nil
}

// Shutdown stops background fetchers.
func (s *Server) Shutdown() {
	s.ical.Stop()
	s.weather.Stop()
	s.snowday.Stop()
	s.tide.Stop()
	s.baseball.Stop()
}

func (s *Server) restartFetchers(c types.Config) {
	s.applyFetcherConfig(s.rootCtx, c, true)
}

// applyFetcherConfig starts fetchers whose widgets are enabled and stops ones
// whose widgets are disabled. When broadcastClears is true, disabled widgets
// also emit a clearing frame so any connected clients drop stale snapshots.
func (s *Server) applyFetcherConfig(ctx context.Context, c types.Config, broadcastClears bool) {
	if c.Display.CalendarEnabled {
		s.ical.Start(ctx, c.Calendars, time.Duration(c.Display.CalendarRefreshSeconds)*time.Second)
	} else {
		s.ical.Stop()
		if broadcastClears {
			s.hub.Broadcast(Frame{Type: "calendar", Events: []types.Event{}})
		}
	}
	if c.Weather.Enabled {
		s.weather.Start(ctx, c.Weather, time.Duration(c.Display.WeatherRefreshSeconds)*time.Second)
	} else {
		s.weather.Stop()
		if broadcastClears {
			s.hub.Broadcast(Frame{Type: "weather", Weather: nil})
		}
	}
	if c.SnowDay.Enabled {
		s.snowday.Start(ctx, c.SnowDay, 0)
	} else {
		s.snowday.Stop()
		if broadcastClears {
			s.hub.Broadcast(Frame{Type: "snowday", SnowDay: nil})
		}
	}
	if c.Tide.Enabled {
		s.tide.Start(ctx, c.Tide, time.Duration(c.Display.TideRefreshSeconds)*time.Second)
	} else {
		s.tide.Stop()
		if broadcastClears {
			s.hub.Broadcast(Frame{Type: "tide", Tide: nil})
		}
	}
	if c.Baseball.Enabled && c.Baseball.TeamID != 0 {
		// Clear any stale snapshot (e.g. a different team) before the new
		// fetch lands so connected clients don't keep showing old data.
		if broadcastClears {
			s.hub.Broadcast(Frame{Type: "baseball", Baseball: nil})
		}
		s.baseball.Start(ctx, c.Baseball, time.Duration(c.Display.BaseballRefreshSeconds)*time.Second)
	} else {
		s.baseball.Stop()
		if broadcastClears {
			s.hub.Broadcast(Frame{Type: "baseball", Baseball: nil})
		}
	}
}

func logging(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		h.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

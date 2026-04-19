package server

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/jpatters/home-calendar/internal/config"
	"github.com/jpatters/home-calendar/internal/ical"
	"github.com/jpatters/home-calendar/internal/snowday"
	"github.com/jpatters/home-calendar/internal/tide"
	"github.com/jpatters/home-calendar/internal/types"
	"github.com/jpatters/home-calendar/internal/weather"
)

type Server struct {
	cfg     *config.Store
	ical    *ical.Fetcher
	weather *weather.Fetcher
	snowday *snowday.Fetcher
	tide    *tide.Fetcher
	hub     *Hub
	rootCtx context.Context
	geocode func(ctx context.Context, query string) ([]weather.GeoResult, error)
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
	s.geocode = func(ctx context.Context, q string) ([]weather.GeoResult, error) {
		return weather.Search(ctx, s.weather.HTTPClient(), weather.DefaultGeocodingURL, q)
	}

	current := cfg.Get()
	s.ical.Start(ctx, current.Calendars, time.Duration(current.Display.CalendarRefreshSeconds)*time.Second)
	s.weather.Start(ctx, current.Weather, time.Duration(current.Display.WeatherRefreshSeconds)*time.Second)
	s.snowday.Start(ctx, current.SnowDay, 0)
	s.tide.Start(ctx, current.Tide, time.Duration(current.Display.TideRefreshSeconds)*time.Second)

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
}

func (s *Server) restartFetchers(c types.Config) {
	s.ical.Start(s.rootCtx, c.Calendars, time.Duration(c.Display.CalendarRefreshSeconds)*time.Second)
	s.weather.Start(s.rootCtx, c.Weather, time.Duration(c.Display.WeatherRefreshSeconds)*time.Second)
	s.snowday.Start(s.rootCtx, c.SnowDay, 0)
	s.tide.Start(s.rootCtx, c.Tide, time.Duration(c.Display.TideRefreshSeconds)*time.Second)
}

func logging(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		h.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

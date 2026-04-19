package server

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/jpatters/home-calendar/internal/config"
	"github.com/jpatters/home-calendar/internal/ical"
	"github.com/jpatters/home-calendar/internal/types"
	"github.com/jpatters/home-calendar/internal/weather"
)

type Server struct {
	cfg      *config.Store
	ical     *ical.Fetcher
	weather  *weather.Fetcher
	hub      *Hub
	rootCtx  context.Context
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

	current := cfg.Get()
	s.ical.Start(ctx, current.Calendars, time.Duration(current.Display.CalendarRefreshSeconds)*time.Second)
	s.weather.Start(ctx, current.Weather, time.Duration(current.Display.WeatherRefreshSeconds)*time.Second)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/config", s.handleGetConfig)
	mux.HandleFunc("PUT /api/config", s.handlePutConfig)
	mux.HandleFunc("GET /api/calendar/events", s.handleGetEvents)
	mux.HandleFunc("POST /api/calendar/refresh", s.handleCalendarRefresh)
	mux.HandleFunc("GET /api/weather", s.handleGetWeather)
	mux.HandleFunc("POST /api/weather/refresh", s.handleWeatherRefresh)
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
}

func (s *Server) restartFetchers(c types.Config) {
	s.ical.Start(s.rootCtx, c.Calendars, time.Duration(c.Display.CalendarRefreshSeconds)*time.Second)
	s.weather.Start(s.rootCtx, c.Weather, time.Duration(c.Display.WeatherRefreshSeconds)*time.Second)
}

func logging(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		h.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

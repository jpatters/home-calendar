package server

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jpatters/home-calendar/internal/types"
)

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg.Get()
	writeJSON(w, http.StatusOK, cfg)
}

func (s *Server) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	var incoming types.Config
	if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	saved, err := s.cfg.Replace(incoming)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.restartFetchers(saved)
	s.hub.Broadcast(Frame{Type: "config", Config: &saved})
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) handleGetEvents(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.ical.Events())
}

func (s *Server) handleCalendarRefresh(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg.Get()
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	s.ical.RefreshNow(ctx, cfg.Calendars)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleGetWeather(w http.ResponseWriter, r *http.Request) {
	snap := s.weather.Snapshot()
	if snap == nil {
		writeJSON(w, http.StatusOK, nil)
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (s *Server) handleWeatherRefresh(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg.Get()
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	s.weather.RefreshNow(ctx, cfg.Weather)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleGetSnowDay(w http.ResponseWriter, r *http.Request) {
	snap := s.snowday.Snapshot()
	if snap == nil {
		writeJSON(w, http.StatusOK, nil)
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (s *Server) handleSnowDayRefresh(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg.Get()
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	s.snowday.RefreshNow(ctx, cfg.SnowDay)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if body == nil {
		w.Write([]byte("null"))
		return
	}
	_ = json.NewEncoder(w).Encode(body)
}

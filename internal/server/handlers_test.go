package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpatters/home-calendar/internal/config"
	"github.com/jpatters/home-calendar/internal/ical"
	"github.com/jpatters/home-calendar/internal/snowday"
	"github.com/jpatters/home-calendar/internal/tide"
	"github.com/jpatters/home-calendar/internal/weather"
)

func newTestStore(t *testing.T, mutate func(*config.Store)) *config.Store {
	t.Helper()
	dir := t.TempDir()
	store, err := config.Open(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatalf("config.Open: %v", err)
	}
	if mutate != nil {
		mutate(store)
	}
	return store
}

func TestWeatherRefreshReturns409WhenDisabled(t *testing.T) {
	store := newTestStore(t, func(s *config.Store) {
		cfg := s.Get()
		cfg.Weather.Enabled = false
		if _, err := s.Replace(cfg); err != nil {
			t.Fatalf("Replace: %v", err)
		}
	})
	srv := &Server{cfg: store, weather: weather.New(nil)}
	req := httptest.NewRequest(http.MethodPost, "/api/weather/refresh", nil)
	rec := httptest.NewRecorder()
	srv.handleWeatherRefresh(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestTideRefreshReturns409WhenDisabled(t *testing.T) {
	store := newTestStore(t, func(s *config.Store) {
		cfg := s.Get()
		cfg.Tide.Enabled = false
		if _, err := s.Replace(cfg); err != nil {
			t.Fatalf("Replace: %v", err)
		}
	})
	srv := &Server{cfg: store, tide: tide.New(nil)}
	req := httptest.NewRequest(http.MethodPost, "/api/tide/refresh", nil)
	rec := httptest.NewRecorder()
	srv.handleTideRefresh(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestSnowDayRefreshReturns409WhenDisabled(t *testing.T) {
	store := newTestStore(t, func(s *config.Store) {
		cfg := s.Get()
		cfg.SnowDay.Enabled = false
		if _, err := s.Replace(cfg); err != nil {
			t.Fatalf("Replace: %v", err)
		}
	})
	srv := &Server{cfg: store, snowday: snowday.New(nil)}
	req := httptest.NewRequest(http.MethodPost, "/api/snowday/refresh", nil)
	rec := httptest.NewRecorder()
	srv.handleSnowDayRefresh(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestCalendarRefreshReturns409WhenDisabled(t *testing.T) {
	store := newTestStore(t, func(s *config.Store) {
		cfg := s.Get()
		cfg.Display.CalendarEnabled = false
		if _, err := s.Replace(cfg); err != nil {
			t.Fatalf("Replace: %v", err)
		}
	})
	srv := &Server{cfg: store, ical: ical.New(nil)}
	req := httptest.NewRequest(http.MethodPost, "/api/calendar/refresh", nil)
	rec := httptest.NewRecorder()
	srv.handleCalendarRefresh(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestWeatherGeocodeRejectsEmptyQuery(t *testing.T) {
	s := &Server{geocode: func(context.Context, string) ([]weather.GeoResult, error) {
		t.Fatal("geocode should not be called on empty query")
		return nil, nil
	}}
	req := httptest.NewRequest(http.MethodGet, "/api/weather/geocode?q=", nil)
	rec := httptest.NewRecorder()
	s.handleWeatherGeocode(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestWeatherGeocodeReturnsResults(t *testing.T) {
	want := []weather.GeoResult{
		{Name: "Toronto", Admin1: "Ontario", Country: "Canada", Timezone: "America/Toronto", Latitude: 43.7, Longitude: -79.4},
	}
	s := &Server{geocode: func(_ context.Context, q string) ([]weather.GeoResult, error) {
		if q != "toronto" {
			t.Errorf("expected q=toronto, got %q", q)
		}
		return want, nil
	}}
	req := httptest.NewRequest(http.MethodGet, "/api/weather/geocode?q=toronto", nil)
	rec := httptest.NewRecorder()
	s.handleWeatherGeocode(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	var got []weather.GeoResult
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v; body=%s", err, rec.Body.String())
	}
	if len(got) != 1 || got[0].Name != "Toronto" || got[0].Country != "Canada" {
		t.Fatalf("unexpected body: %+v", got)
	}
}

func TestWeatherGeocodeSurfacesUpstreamErrorAsBadGateway(t *testing.T) {
	s := &Server{geocode: func(context.Context, string) ([]weather.GeoResult, error) {
		return nil, errors.New("upstream:http://secret-internal-host/details")
	}}
	req := httptest.NewRequest(http.MethodGet, "/api/weather/geocode?q=toronto", nil)
	rec := httptest.NewRecorder()
	s.handleWeatherGeocode(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "secret-internal-host") {
		t.Errorf("response body leaked upstream error: %q", rec.Body.String())
	}
}

func TestWeatherGeocodeRejectsOverlongQuery(t *testing.T) {
	s := &Server{geocode: func(context.Context, string) ([]weather.GeoResult, error) {
		t.Fatal("geocode should not be called for overlong query")
		return nil, nil
	}}
	long := strings.Repeat("a", 101)
	req := httptest.NewRequest(http.MethodGet, "/api/weather/geocode?q="+long, nil)
	rec := httptest.NewRecorder()
	s.handleWeatherGeocode(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

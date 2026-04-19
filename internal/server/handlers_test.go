package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jpatters/home-calendar/internal/weather"
)

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

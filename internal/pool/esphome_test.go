package pool_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jpatters/home-calendar/internal/pool"
)

// climateStub serves a canned ESPHome web_server climate response. ESPHome
// returns the temperatures as JSON *strings* (e.g. "25.2"), which is the
// behaviour the parser must handle.
func climateStub(t *testing.T, action, current, target string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/climate/Pool Heater" {
			t.Errorf("expected request path /climate/Pool Heater, got %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"name_id":"climate/Pool Heater","id":"climate-pool_heater","mode":"HEAT","action":%q,"state":%q,"current_temperature":%q,"target_temperature":%q}`,
			action, action, current, target)
	}))
}

func TestSearchParsesTemperaturesAndHeatingAction(t *testing.T) {
	srv := climateStub(t, "HEATING", "25.2", "27.0")
	defer srv.Close()

	snap, err := pool.Search(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if snap == nil {
		t.Fatalf("expected snapshot, got nil")
	}
	if snap.TemperatureC != 25.2 {
		t.Errorf("TemperatureC = %v, want 25.2", snap.TemperatureC)
	}
	if snap.TargetC != 27.0 {
		t.Errorf("TargetC = %v, want 27.0", snap.TargetC)
	}
	if !snap.Heating {
		t.Errorf("Heating = false, want true when action is HEATING")
	}
}

func TestSearchReportsIdleWhenNotHeating(t *testing.T) {
	srv := climateStub(t, "IDLE", "26.9", "27.0")
	defer srv.Close()

	snap, err := pool.Search(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if snap == nil {
		t.Fatalf("expected snapshot, got nil")
	}
	if snap.Heating {
		t.Errorf("Heating = true, want false when action is IDLE")
	}
	if snap.TemperatureC != 26.9 {
		t.Errorf("TemperatureC = %v, want 26.9", snap.TemperatureC)
	}
}

func TestSearchRejectsNonFiniteTemperature(t *testing.T) {
	// ESPHome emits "nan" for current_temperature while the probe is
	// unavailable; ParseFloat accepts it as NaN, so Search must reject it
	// rather than broadcast a bogus reading.
	srv := climateStub(t, "IDLE", "nan", "27.0")
	defer srv.Close()

	snap, err := pool.Search(context.Background(), srv.Client(), srv.URL)
	if err == nil {
		t.Fatalf("expected error for non-finite temperature, got snap=%+v", snap)
	}
	if snap != nil {
		t.Errorf("expected nil snapshot on non-finite temperature, got %+v", snap)
	}
}

func TestSearchReturnsErrorOnUpstreamFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	snap, err := pool.Search(context.Background(), srv.Client(), srv.URL)
	if err == nil {
		t.Fatalf("expected error on 500, got snap=%+v", snap)
	}
	if snap != nil {
		t.Errorf("expected nil snapshot on error, got %+v", snap)
	}
}

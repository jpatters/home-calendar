package tide_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jpatters/home-calendar/internal/tide"
	"github.com/jpatters/home-calendar/internal/types"
)

// marineStub serves a canned Open-Meteo marine response with the supplied
// hourly times and sea-level heights.
func marineStub(t *testing.T, times []string, heights []float64) *httptest.Server {
	t.Helper()
	body := map[string]any{
		"timezone": "UTC",
		"hourly": map[string]any{
			"time":                 times,
			"sea_level_height_msl": heights,
		},
		"hourly_units": map[string]any{
			"sea_level_height_msl": "metre",
		},
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("hourly"); !strings.Contains(got, "sea_level_height_msl") {
			t.Errorf("expected hourly to include sea_level_height_msl, got %q", got)
		}
		if r.URL.Query().Get("latitude") == "" || r.URL.Query().Get("longitude") == "" {
			t.Errorf("expected latitude and longitude query params")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
}

func hourlyTimes(start time.Time, count int) []string {
	out := make([]string, count)
	for i := range out {
		out[i] = start.Add(time.Duration(i) * time.Hour).Format("2006-01-02T15:04")
	}
	return out
}

func mustParse(t *testing.T, layout, v string) time.Time {
	t.Helper()
	ts, err := time.Parse(layout, v)
	if err != nil {
		t.Fatalf("parse %q: %v", v, err)
	}
	return ts
}

func TestSearchDetectsHighAndLowTides(t *testing.T) {
	// A 10-hour wave: low around hour 5 (h=0.1), highs at hour 2 (h=1.2)
	// and hour 8 (h=1.1).
	heights := []float64{0.5, 0.8, 1.2, 0.8, 0.4, 0.1, 0.3, 0.7, 1.1, 0.9}
	start := mustParse(t, "2006-01-02T15:04", "2026-04-19T00:00")
	srv := marineStub(t, hourlyTimes(start, len(heights)), heights)
	defer srv.Close()

	// "now" is before any event so all three should be in the future.
	now := start.Add(-1 * time.Hour)
	snap, err := tide.Search(context.Background(), srv.Client(), srv.URL, types.Tide{
		Latitude: 49.28, Longitude: -123.12, Units: "metric",
	}, now)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if snap == nil {
		t.Fatalf("expected snapshot, got nil")
	}
	if len(snap.Events) != 3 {
		t.Fatalf("expected 3 events, got %d: %+v", len(snap.Events), snap.Events)
	}
	checks := []struct {
		wantType   string
		wantOffset int
		wantHeight float64
	}{
		{"high", 2, 1.2},
		{"low", 5, 0.1},
		{"high", 8, 1.1},
	}
	for i, c := range checks {
		got := snap.Events[i]
		if got.Type != c.wantType {
			t.Errorf("event %d: type = %q, want %q", i, got.Type, c.wantType)
		}
		wantTime := start.Add(time.Duration(c.wantOffset) * time.Hour)
		if !got.Time.Equal(wantTime) {
			t.Errorf("event %d: time = %v, want %v", i, got.Time, wantTime)
		}
		if got.HeightMeters != c.wantHeight {
			t.Errorf("event %d: height = %v, want %v", i, got.HeightMeters, c.wantHeight)
		}
	}
}

func TestSearchTreatsPlateauAsSingleEvent(t *testing.T) {
	// Plateau at hours 2-3 (both 1.0), then descent. Expect one high event
	// at the midpoint (hour 2.5).
	heights := []float64{0.2, 0.7, 1.0, 1.0, 0.5, 0.2}
	start := mustParse(t, "2006-01-02T15:04", "2026-04-19T00:00")
	srv := marineStub(t, hourlyTimes(start, len(heights)), heights)
	defer srv.Close()

	snap, err := tide.Search(context.Background(), srv.Client(), srv.URL, types.Tide{
		Latitude: 1, Longitude: 1,
	}, start.Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if snap == nil || len(snap.Events) != 1 {
		t.Fatalf("expected exactly one event, got %+v", snap)
	}
	ev := snap.Events[0]
	if ev.Type != "high" {
		t.Errorf("type = %q, want high", ev.Type)
	}
	wantMid := start.Add(2*time.Hour + 30*time.Minute)
	if !ev.Time.Equal(wantMid) {
		t.Errorf("time = %v, want midpoint of plateau %v", ev.Time, wantMid)
	}
	if ev.HeightMeters != 1.0 {
		t.Errorf("height = %v, want 1.0", ev.HeightMeters)
	}
}

func TestSearchFiltersEventsBeforeNow(t *testing.T) {
	// Three events: high at 2, low at 5, high at 8. "now" is at hour 4 so
	// only the low (hour 5) and the high (hour 8) should be reported.
	heights := []float64{0.5, 0.8, 1.2, 0.8, 0.4, 0.1, 0.3, 0.7, 1.1, 0.9}
	start := mustParse(t, "2006-01-02T15:04", "2026-04-19T00:00")
	srv := marineStub(t, hourlyTimes(start, len(heights)), heights)
	defer srv.Close()

	now := start.Add(4 * time.Hour)
	snap, err := tide.Search(context.Background(), srv.Client(), srv.URL, types.Tide{
		Latitude: 1, Longitude: 1,
	}, now)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if snap == nil || len(snap.Events) != 2 {
		t.Fatalf("expected 2 future events, got %+v", snap)
	}
	if snap.Events[0].Type != "low" || snap.Events[1].Type != "high" {
		t.Errorf("types = [%s, %s], want [low, high]",
			snap.Events[0].Type, snap.Events[1].Type)
	}
	for i, ev := range snap.Events {
		if ev.Time.Before(now) {
			t.Errorf("event %d at %v is before now %v", i, ev.Time, now)
		}
	}
}

func TestSearchInterpolatesCurrentHeight(t *testing.T) {
	// Heights 0 (@0h) -> 2 (@1h). "now" at 30 minutes → current should be 1.
	heights := []float64{0.0, 2.0, 2.0, 0.0}
	start := mustParse(t, "2006-01-02T15:04", "2026-04-19T00:00")
	srv := marineStub(t, hourlyTimes(start, len(heights)), heights)
	defer srv.Close()

	now := start.Add(30 * time.Minute)
	snap, err := tide.Search(context.Background(), srv.Client(), srv.URL, types.Tide{
		Latitude: 1, Longitude: 1,
	}, now)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if snap == nil {
		t.Fatalf("expected snapshot")
	}
	if snap.CurrentMeters != 1.0 {
		t.Errorf("CurrentMeters = %v, want 1.0 between two hourly samples", snap.CurrentMeters)
	}
}

func TestSearchReturnsErrorOnUpstreamFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	snap, err := tide.Search(context.Background(), srv.Client(), srv.URL, types.Tide{
		Latitude: 1, Longitude: 1,
	}, time.Now())
	if err == nil {
		t.Fatalf("expected error on 500, got snap=%+v", snap)
	}
	if snap != nil {
		t.Errorf("expected nil snapshot on error, got %+v", snap)
	}
}

func TestSearchSurvivesTrailingPlateau(t *testing.T) {
	// Trailing plateau at the end of the series must not panic.
	heights := []float64{0.5, 1.0, 1.0, 1.0}
	start := mustParse(t, "2006-01-02T15:04", "2026-04-19T00:00")
	srv := marineStub(t, hourlyTimes(start, len(heights)), heights)
	defer srv.Close()

	snap, err := tide.Search(context.Background(), srv.Client(), srv.URL, types.Tide{
		Latitude: 1, Longitude: 1,
	}, start.Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if snap == nil {
		t.Fatalf("expected snapshot, got nil")
	}
	// Without a trailing neighbour we cannot confirm a turning point, so no
	// events should be emitted.
	if len(snap.Events) != 0 {
		t.Errorf("expected 0 events for trailing-plateau series, got %d: %+v",
			len(snap.Events), snap.Events)
	}
}

func TestSearchReturnsNoEventsWhenHourlyMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"timezone":"UTC","hourly":{"time":[],"sea_level_height_msl":[]}}`)
	}))
	defer srv.Close()

	snap, err := tide.Search(context.Background(), srv.Client(), srv.URL, types.Tide{
		Latitude: 1, Longitude: 1,
	}, time.Now())
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if snap == nil {
		t.Fatalf("expected snapshot with empty events, got nil")
	}
	if len(snap.Events) != 0 {
		t.Errorf("expected 0 events, got %d", len(snap.Events))
	}
}

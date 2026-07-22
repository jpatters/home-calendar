package tide_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jpatters/home-calendar/internal/tide"
	"github.com/jpatters/home-calendar/internal/types"
)

const (
	testStationID   = "5cebf1e33d0f4a073c4bc221"
	testStationCode = "01710"
)

type sample struct {
	at    time.Time
	value float64
}

// chsStub serves CHS IWLS-shaped responses for a single station. It enforces
// the same request contracts the live service does — station lookups for an
// unknown code answer 200 with an empty array, timestamps must be RFC3339 UTC
// (date-only is rejected with 400), and each series has a maximum window,
// reported by IWLS as allowedPeriodInDays: 366 days for wlp-hilo, 7 for wlp.
// A test therefore only passes if the requests we build would also be
// accepted by the real API.
func chsStub(t *testing.T, known bool, hilo, wlp []sample) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("GET /stations", func(w http.ResponseWriter, r *http.Request) {
		if !known || r.URL.Query().Get("code") != testStationCode {
			writeStubJSON(w, []any{})
			return
		}
		writeStubJSON(w, []map[string]any{{
			"id":           testStationID,
			"code":         testStationCode,
			"officialName": "Canoe Cove",
			"latitude":     46.149224,
			"longitude":    -63.303736,
		}})
	})

	mux.HandleFunc("GET /stations/{id}/data", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") != testStationID {
			stubError(w, http.StatusNotFound, "Could not find station")
			return
		}
		q := r.URL.Query()
		from, okFrom := stubTime(q.Get("from"))
		to, okTo := stubTime(q.Get("to"))
		if !okFrom || !okTo {
			stubError(w, http.StatusBadRequest, "Wrong parameter format")
			return
		}
		var data []sample
		switch q.Get("time-series-code") {
		case "wlp-hilo":
			if to.Sub(from) > 366*24*time.Hour {
				stubError(w, http.StatusBadRequest, "date interval should not be bigger than 366 days")
				return
			}
			data = hilo
		case "wlp":
			if to.Sub(from) > 7*24*time.Hour {
				stubError(w, http.StatusBadRequest, "date interval should not be bigger than 7 days")
				return
			}
			data = wlp
		default:
			stubError(w, http.StatusNotFound, "Time series code not found in enum")
			return
		}
		writeStubJSON(w, samplesIn(data, from, to))
	})

	return httptest.NewServer(mux)
}

// stubTime accepts only the format the live API accepts: RFC3339 with an
// explicit UTC zone and seconds.
func stubTime(v string) (time.Time, bool) {
	ts, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}, false
	}
	return ts.UTC(), true
}

func samplesIn(all []sample, from, to time.Time) []map[string]any {
	out := []map[string]any{}
	for _, s := range all {
		if s.at.Before(from) || s.at.After(to) {
			continue
		}
		out = append(out, map[string]any{
			"eventDate":    s.at.UTC().Format(time.RFC3339),
			"value":        s.value,
			"qcFlagCode":   "1",
			"reviewed":     false,
			"timeSeriesId": "5d9dd7da33a9f593161c415a",
		})
	}
	return out
}

func writeStubJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

func stubError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"status":    fmt.Sprintf("%d", status),
		"message":   message,
		"errors":    []string{message},
	})
}

// extrema builds an alternating high/low series starting at `start`, spaced
// every 6h12m the way a semidiurnal station behaves.
func extrema(start time.Time, values []float64) []sample {
	out := make([]sample, len(values))
	for i, v := range values {
		out[i] = sample{at: start.Add(time.Duration(i) * (6*time.Hour + 12*time.Minute)), value: v}
	}
	return out
}

// rampWLP builds a continuous prediction series on a 15-minute grid whose
// value rises linearly with time, so the height at any instant is exactly
// 2.0 + 4.0*hoursSinceStart.
func rampWLP(start time.Time, span time.Duration) []sample {
	out := []sample{}
	for d := -span; d <= span; d += 15 * time.Minute {
		out = append(out, sample{at: start.Add(d), value: 2.0 + 4.0*d.Hours()})
	}
	return out
}

func canoeCoveTide() types.Tide {
	return types.Tide{
		Enabled:     true,
		StationCode: testStationCode,
		Units:       "metric",
		Timezone:    "America/Halifax",
		Location:    "Canoe Cove",
	}
}

func TestSearchClassifiesHighAndLowFromUnlabelledExtrema(t *testing.T) {
	// CHS returns turning points with no high/low marker — only a value and a
	// time. These are real Canoe Cove predictions.
	start := time.Date(2026, 7, 22, 2, 2, 0, 0, time.UTC)
	hilo := extrema(start, []float64{1.162, 2.361, 0.871, 2.151, 1.380, 2.307})
	srv := chsStub(t, true, hilo, rampWLP(start, 48*time.Hour))
	defer srv.Close()

	now := start.Add(time.Minute)
	snap, err := tide.Search(context.Background(), srv.Client(), srv.URL, canoeCoveTide(), now)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if snap == nil {
		t.Fatalf("expected snapshot, got nil")
	}

	want := []types.TideEvent{
		{Time: hilo[1].at, Type: "high", HeightMeters: 2.361},
		{Time: hilo[2].at, Type: "low", HeightMeters: 0.871},
		{Time: hilo[3].at, Type: "high", HeightMeters: 2.151},
		{Time: hilo[4].at, Type: "low", HeightMeters: 1.380},
	}
	if len(snap.Events) != len(want) {
		t.Fatalf("got %d events, want %d: %+v", len(snap.Events), len(want), snap.Events)
	}
	for i, w := range want {
		got := snap.Events[i]
		if got.Type != w.Type {
			t.Errorf("event %d: type = %q, want %q", i, got.Type, w.Type)
		}
		if !got.Time.Equal(w.Time) {
			t.Errorf("event %d: time = %v, want %v", i, got.Time, w.Time)
		}
		if got.HeightMeters != w.HeightMeters {
			t.Errorf("event %d: height = %v, want %v", i, got.HeightMeters, w.HeightMeters)
		}
	}
}

func TestSearchExcludesEventsAlreadyPast(t *testing.T) {
	start := time.Date(2026, 7, 22, 2, 2, 0, 0, time.UTC)
	hilo := extrema(start, []float64{1.162, 2.361, 0.871, 2.151, 1.380, 2.307})
	srv := chsStub(t, true, hilo, rampWLP(start, 48*time.Hour))
	defer srv.Close()

	// One minute after the third turning point.
	now := hilo[2].at.Add(time.Minute)
	snap, err := tide.Search(context.Background(), srv.Client(), srv.URL, canoeCoveTide(), now)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if snap == nil {
		t.Fatalf("expected snapshot, got nil")
	}
	if len(snap.Events) != 2 {
		t.Fatalf("got %d events, want 2: %+v", len(snap.Events), snap.Events)
	}
	if snap.Events[0].Type != "high" || snap.Events[1].Type != "low" {
		t.Errorf("types = [%s %s], want [high low]", snap.Events[0].Type, snap.Events[1].Type)
	}
	for i, ev := range snap.Events {
		if ev.Time.Before(now) {
			t.Errorf("event %d at %v is before now %v", i, ev.Time, now)
		}
	}
}

func TestSearchClassifiesEventsThroughEndOfHorizon(t *testing.T) {
	// A turning point can only be classified by comparing it with the points
	// either side of it, so the fetch must look beyond the horizon it reports.
	// Without that, the final days silently lose their events.
	start := time.Date(2026, 7, 22, 2, 2, 0, 0, time.UTC)
	values := make([]float64, 60) // ~15 days of semidiurnal extrema
	for i := range values {
		if i%2 == 0 {
			values[i] = 0.9
		} else {
			values[i] = 2.3
		}
	}
	hilo := extrema(start, values)
	srv := chsStub(t, true, hilo, rampWLP(start, 48*time.Hour))
	defer srv.Close()

	now := start.Add(time.Minute)
	snap, err := tide.Search(context.Background(), srv.Client(), srv.URL, canoeCoveTide(), now)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if snap == nil || len(snap.Events) == 0 {
		t.Fatalf("expected events, got %+v", snap)
	}

	for i, ev := range snap.Events {
		if ev.Type != "high" && ev.Type != "low" {
			t.Fatalf("event %d has no classification: %+v", i, ev)
		}
		if i > 0 && ev.Type == snap.Events[i-1].Type {
			t.Errorf("events %d and %d are both %q; tides must alternate", i-1, i, ev.Type)
		}
	}

	last := snap.Events[len(snap.Events)-1].Time
	if last.Before(now.Add(7 * 24 * time.Hour)) {
		t.Errorf("last event %v is less than 7 days out from %v; the modal pages by day and needs a week of data", last, now)
	}
}

func TestSearchReportsCurrentHeightFromPredictionSeries(t *testing.T) {
	start := time.Date(2026, 7, 22, 2, 2, 0, 0, time.UTC)
	hilo := extrema(start, []float64{1.162, 2.361, 0.871, 2.151, 1.380, 2.307})
	srv := chsStub(t, true, hilo, rampWLP(start, 48*time.Hour))
	defer srv.Close()

	// Deliberately between two samples of the 15-minute grid, so the reading
	// has to be interpolated rather than read off a sample.
	now := start.Add(22 * time.Minute)
	snap, err := tide.Search(context.Background(), srv.Client(), srv.URL, canoeCoveTide(), now)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if snap == nil {
		t.Fatalf("expected snapshot, got nil")
	}
	want := 2.0 + 4.0*(22.0/60.0) // rampWLP is linear in time
	if math.Abs(snap.CurrentMeters-want) > 1e-6 {
		t.Errorf("CurrentMeters = %v, want %v interpolated between the 15m and 30m samples", snap.CurrentMeters, want)
	}
}

func TestSearchReportsConfiguredTimezone(t *testing.T) {
	// CHS timestamps everything in UTC and reports no timezone of its own, so
	// the snapshot has to carry the configured one through to the display.
	start := time.Date(2026, 7, 22, 2, 2, 0, 0, time.UTC)
	hilo := extrema(start, []float64{1.162, 2.361, 0.871, 2.151, 1.380, 2.307})
	srv := chsStub(t, true, hilo, rampWLP(start, 48*time.Hour))
	defer srv.Close()

	snap, err := tide.Search(context.Background(), srv.Client(), srv.URL, canoeCoveTide(), start.Add(time.Minute))
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if snap.Timezone != "America/Halifax" {
		t.Errorf("Timezone = %q, want the configured America/Halifax", snap.Timezone)
	}
}

func TestSearchWithNoStationSelectedContactsNothing(t *testing.T) {
	// A config carried over from the coordinate-based source has no station
	// code. The refresh loop keeps running, so this must fail locally rather
	// than send a meaningless request to CHS every hour.
	var reqs atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqs.Add(1)
		writeStubJSON(w, []any{})
	}))
	defer srv.Close()

	cfg := canoeCoveTide()
	cfg.StationCode = ""
	snap, err := tide.Search(context.Background(), srv.Client(), srv.URL, cfg, time.Now())
	if err == nil {
		t.Fatalf("expected an error with no station selected, got %+v", snap)
	}
	if got := reqs.Load(); got != 0 {
		t.Errorf("made %d upstream requests with no station selected, want 0", got)
	}
}

func TestSearchFailsWhenStationCodeMatchesNoStation(t *testing.T) {
	// The station lookup answers 200 with an empty array for an unknown code.
	// That must not be mistaken for "this station has no tides".
	start := time.Date(2026, 7, 22, 2, 2, 0, 0, time.UTC)
	srv := chsStub(t, false, nil, nil)
	defer srv.Close()

	snap, err := tide.Search(context.Background(), srv.Client(), srv.URL, canoeCoveTide(), start)
	if err == nil {
		t.Fatalf("expected an error for an unresolvable station, got snapshot %+v", snap)
	}
	if snap != nil {
		t.Errorf("expected nil snapshot alongside the error, got %+v", snap)
	}
}

func TestSearchFailsWhenPredictionSeriesHasNoCoverage(t *testing.T) {
	// A station can hold high/low predictions for a window its continuous
	// series does not cover. Reporting a current height of zero there would
	// read as a real reading, so the whole refresh must fail and leave the
	// previous snapshot in place instead.
	start := time.Date(2026, 7, 22, 2, 2, 0, 0, time.UTC)
	hilo := extrema(start, []float64{1.162, 2.361, 0.871, 2.151, 1.380, 2.307})
	srv := chsStub(t, true, hilo, nil)
	defer srv.Close()

	snap, err := tide.Search(context.Background(), srv.Client(), srv.URL, canoeCoveTide(), start.Add(time.Minute))
	if err == nil {
		t.Fatalf("expected an error when the prediction series is empty, got %+v", snap)
	}
	if snap != nil {
		t.Errorf("expected nil snapshot alongside the error, got %+v", snap)
	}
}

func TestSearchSurfacesUpstreamFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	snap, err := tide.Search(context.Background(), srv.Client(), srv.URL, canoeCoveTide(), time.Now())
	if err == nil {
		t.Fatalf("expected error on 500, got snap=%+v", snap)
	}
	if snap != nil {
		t.Errorf("expected nil snapshot on error, got %+v", snap)
	}
}

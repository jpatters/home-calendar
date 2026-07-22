package tide_test

import (
	"context"
	"encoding/json"
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
// the request contracts the live service enforces — timestamps must be RFC3339
// UTC (date-only is rejected with 400), and each series has a maximum window,
// reported by IWLS as allowedPeriodInDays: 366 days for wlp-hilo, 7 for wlp.
// It does not police every parameter, so it narrows rather than eliminates the
// gap between passing here and working live.
func chsStub(t *testing.T, hilo, wlp []sample) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("GET /stations", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("code") != testStationCode {
			writeStubJSON(w, []any{})
			return
		}
		writeStubJSON(w, []map[string]any{{
			"id":           testStationID,
			"code":         testStationCode,
			"officialName": "Canoe Cove",
		}})
	})

	mux.HandleFunc("GET /stations/{id}/data", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") != testStationID {
			stubError(w, http.StatusNotFound)
			return
		}
		q := r.URL.Query()
		from, okFrom := stubTime(q.Get("from"))
		to, okTo := stubTime(q.Get("to"))
		if !okFrom || !okTo {
			stubError(w, http.StatusBadRequest)
			return
		}
		var data []sample
		switch q.Get("time-series-code") {
		case "wlp-hilo":
			if to.Sub(from) > 366*24*time.Hour {
				stubError(w, http.StatusBadRequest)
				return
			}
			data = hilo
		case "wlp":
			if to.Sub(from) > 7*24*time.Hour {
				stubError(w, http.StatusBadRequest)
				return
			}
			data = wlp
		default:
			stubError(w, http.StatusNotFound)
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
			"eventDate": s.at.UTC().Format(time.RFC3339),
			"value":     s.value,
		})
	}
	return out
}

func writeStubJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

func stubError(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": status})
}

// extrema builds an alternating high/low series spaced every `every`.
func extrema(start time.Time, count int, every time.Duration) []sample {
	out := make([]sample, count)
	for i := range out {
		v := 0.871
		if i%2 != 0 {
			v = 2.361
		}
		out[i] = sample{at: start.Add(time.Duration(i) * every), value: v}
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
		Location:    "Canoe Cove",
	}
}

// canoeCoveStub is the common fixture: a semidiurnal series of real Canoe Cove
// turning points around `start`, with continuous levels covering it.
func canoeCoveStub(t *testing.T, start time.Time) (*httptest.Server, []sample) {
	t.Helper()
	hilo := []sample{}
	for i, v := range []float64{1.162, 2.361, 0.871, 2.151, 1.380, 2.307} {
		hilo = append(hilo, sample{at: start.Add(time.Duration(i) * (6*time.Hour + 12*time.Minute)), value: v})
	}
	return chsStub(t, hilo, rampWLP(start, 48*time.Hour)), hilo
}

func TestSearchClassifiesHighAndLowFromUnlabelledExtrema(t *testing.T) {
	// CHS returns turning points with no high/low marker — only a value and a
	// time. These are real Canoe Cove predictions.
	start := time.Date(2026, 7, 22, 2, 2, 0, 0, time.UTC)
	srv, hilo := canoeCoveStub(t, start)
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
		{Time: hilo[5].at, Type: "high", HeightMeters: 2.307},
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
	if snap.Units != "metric" {
		t.Errorf("Units = %q, want the configured metric", snap.Units)
	}
}

func TestSearchExcludesEventsAlreadyPast(t *testing.T) {
	start := time.Date(2026, 7, 22, 2, 2, 0, 0, time.UTC)
	srv, hilo := canoeCoveStub(t, start)
	defer srv.Close()

	// One minute after the third turning point.
	now := hilo[2].at.Add(time.Minute)
	snap, err := tide.Search(context.Background(), srv.Client(), srv.URL, canoeCoveTide(), now)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(snap.Events) != 3 {
		t.Fatalf("got %d events, want 3: %+v", len(snap.Events), snap.Events)
	}
	for i, ev := range snap.Events {
		if ev.Time.Before(now) {
			t.Errorf("event %d at %v is before now %v", i, ev.Time, now)
		}
	}
}

func TestSearchReportsExactlyOneHorizonOfEvents(t *testing.T) {
	// Turning points every 6h starting at now, so 8 days of horizon holds
	// precisely 33 of them (k=0..32). Anything that widens the horizon, drops
	// the trim at either end, or loses the event sitting on the boundary
	// changes this count.
	now := time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC)
	const step = 6 * time.Hour
	points := extrema(now.Add(-4*step), 60, step) // starts a day before now
	srv := chsStub(t, points, rampWLP(now, 48*time.Hour))
	defer srv.Close()

	snap, err := tide.Search(context.Background(), srv.Client(), srv.URL, canoeCoveTide(), now)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	horizonEnd := now.Add(8 * 24 * time.Hour)
	if len(snap.Events) != 33 {
		t.Fatalf("got %d events, want 33 for an 8-day horizon at %v spacing; last=%v",
			len(snap.Events), step, snap.Events[len(snap.Events)-1].Time)
	}
	if !snap.Events[0].Time.Equal(now) {
		t.Errorf("first event at %v, want the one falling exactly on now (%v)", snap.Events[0].Time, now)
	}
	if !snap.Events[32].Time.Equal(horizonEnd) {
		t.Errorf("last event at %v, want the one falling exactly on the horizon edge (%v)", snap.Events[32].Time, horizonEnd)
	}
	for i, ev := range snap.Events {
		if ev.Type != "high" && ev.Type != "low" {
			t.Fatalf("event %d unclassified: %+v", i, ev)
		}
		if i > 0 && ev.Type == snap.Events[i-1].Type {
			t.Errorf("events %d and %d are both %q; tides alternate", i-1, i, ev.Type)
		}
	}
}

func TestSearchClassifiesFinalEventOfAStationsCoverage(t *testing.T) {
	// A station's published predictions can stop inside the queried window —
	// several stations offered here are discontinued gauges. The last point
	// then has no following neighbour, but it is still a real turning point
	// and must be reported rather than dropped.
	now := time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC)
	const step = 6 * time.Hour
	// Coverage stops two days out, well inside the 8-day horizon.
	points := extrema(now, 9, step)
	srv := chsStub(t, points, rampWLP(now, 48*time.Hour))
	defer srv.Close()

	snap, err := tide.Search(context.Background(), srv.Client(), srv.URL, canoeCoveTide(), now)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(snap.Events) != len(points) {
		t.Fatalf("got %d events, want all %d published turning points: %+v",
			len(snap.Events), len(points), snap.Events)
	}
	last := snap.Events[len(snap.Events)-1]
	if !last.Time.Equal(points[len(points)-1].at) {
		t.Errorf("last event at %v, want the final published point %v", last.Time, points[len(points)-1].at)
	}
	if last.Type != "low" {
		t.Errorf("final point (%.3f m, following %.3f m) classified %q, want low",
			points[8].value, points[7].value, last.Type)
	}
}

func TestSearchInterpolatesCurrentHeightBetweenSamples(t *testing.T) {
	start := time.Date(2026, 7, 22, 2, 2, 0, 0, time.UTC)
	srv, _ := canoeCoveStub(t, start)
	defer srv.Close()

	// Deliberately between two samples of the 15-minute grid, so the reading
	// has to be interpolated rather than read off a sample.
	now := start.Add(22 * time.Minute)
	snap, err := tide.Search(context.Background(), srv.Client(), srv.URL, canoeCoveTide(), now)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	want := 2.0 + 4.0*(22.0/60.0) // rampWLP is linear in time
	if math.Abs(snap.CurrentMeters-want) > 1e-6 {
		t.Errorf("CurrentMeters = %v, want %v interpolated between the 15m and 30m samples", snap.CurrentMeters, want)
	}
}

func TestSearchFailsWhenStationCodeMatchesNoStation(t *testing.T) {
	// The station lookup answers 200 with an empty array for an unknown code.
	// That must not be mistaken for "this station has no tides".
	start := time.Date(2026, 7, 22, 2, 2, 0, 0, time.UTC)
	srv, _ := canoeCoveStub(t, start)
	defer srv.Close()

	cfg := canoeCoveTide()
	cfg.StationCode = "99999"
	snap, err := tide.Search(context.Background(), srv.Client(), srv.URL, cfg, start)
	if err == nil {
		t.Fatalf("expected an error for an unresolvable station, got snapshot %+v", snap)
	}
	if snap != nil {
		t.Errorf("expected nil snapshot alongside the error, got %+v", snap)
	}
}

func TestSearchRejectsAStationWhoseCodeDoesNotMatch(t *testing.T) {
	// Everything downstream assumes the code identifies the station exactly.
	// If the upstream filter ever matched loosely — or were ignored — taking
	// whatever came back would show another harbour's tides under this
	// station's name, which is the failure this source was chosen to end.
	start := time.Date(2026, 7, 22, 2, 2, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeStubJSON(w, []map[string]any{{
			"id":           "5cebf1e33d0f4a073c4bc999",
			"code":         "01700",
			"officialName": "Charlottetown",
		}})
	}))
	defer srv.Close()

	snap, err := tide.Search(context.Background(), srv.Client(), srv.URL, canoeCoveTide(), start)
	if err == nil {
		t.Fatalf("expected an error when the returned station is a different one, got %+v", snap)
	}
	if snap != nil {
		t.Errorf("expected nil snapshot alongside the error, got %+v", snap)
	}
}

func TestSearchFailsWhenSeriesHaveNoCoverage(t *testing.T) {
	// A station can hold predictions for a window one of its series does not
	// cover. Publishing a snapshot from that would replace good data with an
	// empty event list or a zero reading, both of which look real on screen.
	start := time.Date(2026, 7, 22, 2, 2, 0, 0, time.UTC)
	_, hilo := canoeCoveStub(t, start)

	for _, tc := range []struct {
		name      string
		hilo, wlp []sample
	}{
		{"no high/low predictions", nil, rampWLP(start, 48*time.Hour)},
		{"no water level predictions", hilo, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := chsStub(t, tc.hilo, tc.wlp)
			defer srv.Close()

			snap, err := tide.Search(context.Background(), srv.Client(), srv.URL, canoeCoveTide(), start.Add(time.Minute))
			if err == nil {
				t.Fatalf("expected an error, got %+v", snap)
			}
			if snap != nil {
				t.Errorf("expected nil snapshot alongside the error, got %+v", snap)
			}
		})
	}
}

func TestSearchFailsLocallyWhenNoStationSelected(t *testing.T) {
	// A config carried over from the coordinate-based source has no station
	// code. The refresh loop keeps running, so this must fail without sending
	// a meaningless request to CHS every hour.
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

func TestFetcherClearsSnapshotWhenStationIsUnset(t *testing.T) {
	// The upgrade path: a config written against the old coordinate-based
	// source arrives with no station code. Any tides already on screen came
	// from the old source and are wrong, so they must be cleared rather than
	// left up.
	start := time.Date(2026, 7, 22, 2, 2, 0, 0, time.UTC)
	srv, _ := canoeCoveStub(t, start)
	defer srv.Close()

	broadcasts := []*types.TideSnapshot{}
	f := tide.New(srv.URL, func(snap *types.TideSnapshot) {
		broadcasts = append(broadcasts, snap)
	})
	f.RefreshNow(context.Background(), canoeCoveTide())
	if f.Snapshot() == nil {
		t.Fatalf("expected a snapshot after refreshing a configured station")
	}

	cfg := canoeCoveTide()
	cfg.StationCode = ""
	f.RefreshNow(context.Background(), cfg)

	if got := f.Snapshot(); got != nil {
		t.Errorf("snapshot survived the station being unset: %+v", got)
	}
	if len(broadcasts) != 2 || broadcasts[1] != nil {
		t.Errorf("expected a nil broadcast clearing the display, got %+v", broadcasts)
	}
}

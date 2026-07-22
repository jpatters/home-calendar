package tide_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jpatters/home-calendar/internal/tide"
)

type stubStation struct {
	Code      string
	Name      string
	Series    []string
	Type      string
	Operating bool
}

// stationsStub serves the CHS station listing. The live listing carries no
// per-station series array — that lives on the separate metadata endpoint — so
// the only way to narrow by series is the time-series-code query parameter,
// and this reproduces exactly that. Stations are emitted with their lifecycle
// fields so a filter on those has something to catch on.
func stationsStub(t *testing.T, stations []stubStation, reqs *atomic.Int32) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /stations", func(w http.ResponseWriter, r *http.Request) {
		reqs.Add(1)
		want := r.URL.Query().Get("time-series-code")
		out := []map[string]any{}
		for i, s := range stations {
			if want != "" && !hasSeries(s, want) {
				continue
			}
			out = append(out, map[string]any{
				"id":           fmt.Sprintf("%024d", i),
				"code":         s.Code,
				"officialName": s.Name,
				"type":         s.Type,
				"operating":    s.Operating,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})
	return httptest.NewServer(mux)
}

func hasSeries(s stubStation, code string) bool {
	return slices.Contains(s.Series, code)
}

func bothSeries() []string { return []string{"wlp", "wlp-hilo"} }

// active describes a permanent, currently-operating station.
func active(code, name string) stubStation {
	return stubStation{Code: code, Name: name, Series: bothSeries(), Type: "PERMANENT", Operating: true}
}

func codesOf(results []tide.StationResult) []string {
	out := make([]string, len(results))
	for i, r := range results {
		out[i] = r.Code
	}
	return out
}

func TestStationSearchMatchesOnNameAndCode(t *testing.T) {
	var reqs atomic.Int32
	srv := stationsStub(t, []stubStation{
		active("01710", "Canoe Cove"),
		active("01700", "Charlottetown"),
		active("07735", "Vancouver"),
	}, &reqs)
	defer srv.Close()

	dir := tide.NewDirectory(srv.Client(), srv.URL)

	byName, err := dir.Search(context.Background(), "canoe")
	if err != nil {
		t.Fatalf("Search by name: %v", err)
	}
	if len(byName) != 1 || byName[0].Name != "Canoe Cove" || byName[0].Code != "01710" {
		t.Fatalf("search %q returned %+v, want just Canoe Cove", "canoe", byName)
	}

	byCode, err := dir.Search(context.Background(), "01710")
	if err != nil {
		t.Fatalf("Search by code: %v", err)
	}
	if len(byCode) != 1 || byCode[0].Code != "01710" {
		t.Fatalf("search %q returned %+v, want just Canoe Cove", "01710", byCode)
	}
}

func TestStationSearchExcludesStationsMissingEitherSeries(t *testing.T) {
	// The widget needs high/low events *and* a current height, so a station
	// that publishes only one of the two must never be offered.
	var reqs atomic.Int32
	srv := stationsStub(t, []stubStation{
		active("01710", "Canoe Cove"),
		{Code: "09999", Name: "Canoe Rapids", Series: []string{"wlp-hilo"}, Type: "PERMANENT", Operating: true},
		{Code: "09998", Name: "Canoe Narrows", Series: []string{"wlp"}, Type: "PERMANENT", Operating: true},
	}, &reqs)
	defer srv.Close()

	dir := tide.NewDirectory(srv.Client(), srv.URL)
	got, err := dir.Search(context.Background(), "canoe")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 1 || got[0].Code != "01710" {
		t.Fatalf("got %v, want only 01710", codesOf(got))
	}
}

func TestStationSearchOffersDiscontinuedStations(t *testing.T) {
	// Canoe Cove is flagged DISCONTINUED and non-operating, yet publishes
	// predictions years into the future. Those flags describe the physical
	// gauge, not the prediction coverage, so filtering on them would hide the
	// correct station. Only the discontinued station matches the query here,
	// so a filter on either flag makes this fail.
	var reqs atomic.Int32
	srv := stationsStub(t, []stubStation{
		{Code: "01710", Name: "Canoe Cove", Series: bothSeries(), Type: "DISCONTINUED", Operating: false},
		active("01700", "Charlottetown"),
	}, &reqs)
	defer srv.Close()

	dir := tide.NewDirectory(srv.Client(), srv.URL)
	got, err := dir.Search(context.Background(), "canoe cove")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 1 || got[0].Code != "01710" {
		t.Fatalf("got %v, want the discontinued Canoe Cove to still be offered", codesOf(got))
	}
}

func TestStationSearchBoundsResultCount(t *testing.T) {
	// A broad query matches hundreds of the ~1,500 stations; the type-ahead
	// must not try to render them all.
	var reqs atomic.Int32
	stations := make([]stubStation, 300)
	for i := range stations {
		stations[i] = active(fmt.Sprintf("%05d", i), fmt.Sprintf("Harbour %d", i))
	}
	srv := stationsStub(t, stations, &reqs)
	defer srv.Close()

	dir := tide.NewDirectory(srv.Client(), srv.URL)
	got, err := dir.Search(context.Background(), "harbour")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != tide.MaxStationResults {
		t.Errorf("got %d results for 300 matches, want the full bound of %d", len(got), tide.MaxStationResults)
	}
}

func TestStationSearchDoesNotRefetchListPerQuery(t *testing.T) {
	// The full listing is ~830 KB. Typing in the admin panel fires a query per
	// keystroke, so repeats have to be served from what we already hold — and
	// must still return real results, not an emptied cache.
	var reqs atomic.Int32
	srv := stationsStub(t, []stubStation{
		active("01710", "Canoe Cove"),
		active("01700", "Charlottetown"),
	}, &reqs)
	defer srv.Close()

	dir := tide.NewDirectory(srv.Client(), srv.URL)
	if _, err := dir.Search(context.Background(), "c"); err != nil {
		t.Fatalf("first Search: %v", err)
	}
	after := reqs.Load()
	if after == 0 {
		t.Fatalf("expected the first search to fetch the listing")
	}

	var last []tide.StationResult
	for _, q := range []string{"ca", "can", "cano", "canoe"} {
		got, err := dir.Search(context.Background(), q)
		if err != nil {
			t.Fatalf("Search(%q): %v", q, err)
		}
		last = got
	}
	if got := reqs.Load(); got != after {
		t.Errorf("made %d upstream requests across 5 searches, want %d", got, after)
	}
	if len(last) != 1 || last[0].Code != "01710" {
		t.Errorf("repeat search returned %v, want Canoe Cove", codesOf(last))
	}
}

func TestStationSearchSurfacesUpstreamFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	dir := tide.NewDirectory(srv.Client(), srv.URL)
	got, err := dir.Search(context.Background(), "canoe")
	if err == nil {
		t.Fatalf("expected error, got %+v", got)
	}
	if !strings.Contains(err.Error(), "503") {
		t.Errorf("error %q should name the upstream status", err)
	}
}

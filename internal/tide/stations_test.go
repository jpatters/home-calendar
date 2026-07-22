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
	Code   string
	Name   string
	Series []string
}

// stationsStub serves the CHS station listing. The live listing carries no
// per-station series array — that lives on the separate metadata endpoint — so
// the only way to narrow by series is the time-series-code query parameter,
// and this reproduces exactly that.
func stationsStub(t *testing.T, stations []stubStation) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	reqs := &atomic.Int32{}
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
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})
	return httptest.NewServer(mux), reqs
}

func hasSeries(s stubStation, code string) bool {
	return slices.Contains(s.Series, code)
}

func bothSeries() []string { return []string{"wlp", "wlp-hilo"} }

func active(code, name string) stubStation {
	return stubStation{Code: code, Name: name, Series: bothSeries()}
}

func codesOf(results []tide.StationResult) []string {
	out := make([]string, len(results))
	for i, r := range results {
		out[i] = r.Code
	}
	return out
}

func TestStationSearchMatchesOnNameAndCode(t *testing.T) {
	srv, _ := stationsStub(t, []stubStation{
		active("01710", "Canoe Cove"),
		active("01700", "Charlottetown"),
		active("07735", "Vancouver"),
	})
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
	srv, _ := stationsStub(t, []stubStation{
		active("01710", "Canoe Cove"),
		{Code: "09999", Name: "Canoe Rapids", Series: []string{"wlp-hilo"}},
		{Code: "09998", Name: "Canoe Narrows", Series: []string{"wlp"}},
	})
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

func TestStationSearchDoesNotCacheAnEmptyCatalogue(t *testing.T) {
	// If CHS is mid-outage, or renames a series code, the intersection comes
	// back empty. Caching that would mean "No matches found" for a whole day,
	// recoverable only by restarting the server.
	srv, reqs := stationsStub(t, []stubStation{
		{Code: "01710", Name: "Canoe Cove", Series: []string{"some-renamed-series"}},
	})
	defer srv.Close()

	dir := tide.NewDirectory(srv.Client(), srv.URL)
	if _, err := dir.Search(context.Background(), "canoe"); err == nil {
		t.Fatalf("expected an error when no station publishes both series")
	}
	before := reqs.Load()
	if _, err := dir.Search(context.Background(), "canoe"); err == nil {
		t.Fatalf("expected an error on the retry too")
	}
	if reqs.Load() == before {
		t.Errorf("second search served an empty catalogue from cache instead of retrying upstream")
	}
}

func TestStationSearchBoundsResultCount(t *testing.T) {
	// A broad query matches hundreds of the ~1,500 stations; the type-ahead
	// must not try to render them all.
	stations := make([]stubStation, 300)
	for i := range stations {
		stations[i] = active(fmt.Sprintf("%05d", i), fmt.Sprintf("Harbour %d", i))
	}
	srv, _ := stationsStub(t, stations)
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
	srv, reqs := stationsStub(t, []stubStation{
		active("01710", "Canoe Cove"),
		active("01700", "Charlottetown"),
	})
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

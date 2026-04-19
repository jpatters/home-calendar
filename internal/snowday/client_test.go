package snowday_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jpatters/home-calendar/internal/snowday"
	"github.com/jpatters/home-calendar/internal/types"
)

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

// TestFetcherPopulatesSnapshot — exercises the observable contract:
// configure a URL, RefreshNow, then Snapshot returns the decoded data.
func TestFetcherPopulatesSnapshot(t *testing.T) {
	body := loadFixture(t, "canoe-cove.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	f := snowday.New(nil)
	f.RefreshNow(context.Background(), types.SnowDay{URL: srv.URL})

	snap := f.Snapshot()
	if snap == nil {
		t.Fatal("expected a snapshot, got nil")
	}
	if snap.Probability != 0 {
		t.Errorf("Probability: got %d, want 0", snap.Probability)
	}
	if snap.Category != "Low" {
		t.Errorf("Category: got %q, want %q", snap.Category, "Low")
	}
	if snap.Location != "Canoe Cove, PE" {
		t.Errorf("Location: got %q, want %q", snap.Location, "Canoe Cove, PE")
	}
	if snap.RegionName != "Cornwall, PE" {
		t.Errorf("RegionName: got %q, want %q", snap.RegionName, "Cornwall, PE")
	}
	wantMorning, _ := time.Parse(time.RFC3339, "2026-04-20T13:07:42.563-03:00")
	if !snap.MorningTime.Equal(wantMorning) {
		t.Errorf("MorningTime: got %v, want %v", snap.MorningTime, wantMorning)
	}
	if snap.UpdatedAt.IsZero() {
		t.Error("UpdatedAt is zero; expected it to be set")
	}
	if snap.URL != srv.URL {
		t.Errorf("URL: got %q, want %q", snap.URL, srv.URL)
	}
}

// TestFetcherOnHTTPErrorKeepsPriorSnapshot — if the source starts failing,
// we continue serving the last good snapshot.
func TestFetcherOnHTTPErrorKeepsPriorSnapshot(t *testing.T) {
	goodBody := loadFixture(t, "canoe-cove.json")
	fail := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(goodBody)
	}))
	defer srv.Close()

	f := snowday.New(nil)
	cfg := types.SnowDay{URL: srv.URL}

	f.RefreshNow(context.Background(), cfg)
	first := f.Snapshot()
	if first == nil {
		t.Fatal("expected initial snapshot, got nil")
	}

	fail = true
	f.RefreshNow(context.Background(), cfg)

	after := f.Snapshot()
	if after == nil {
		t.Fatal("expected prior snapshot to be preserved on HTTP 500")
	}
	if after.Probability != first.Probability {
		t.Errorf("probability changed after failure: got %d, want %d", after.Probability, first.Probability)
	}
	if !after.UpdatedAt.Equal(first.UpdatedAt) {
		t.Errorf("UpdatedAt changed after failure; expected prior snapshot preserved")
	}
}

// TestFetcherTranslatesPredictionURLToAPIURL — user pastes the URL they see
// in the browser (/prediction/<slug>); we call the API path (/api/query/<slug>).
func TestFetcherTranslatesPredictionURLToAPIURL(t *testing.T) {
	body := loadFixture(t, "canoe-cove.json")
	var mu sync.Mutex
	var requestedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestedPath = r.URL.Path
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	f := snowday.New(nil)
	f.RefreshNow(context.Background(), types.SnowDay{URL: srv.URL + "/prediction/canoe-cove-pe"})

	mu.Lock()
	defer mu.Unlock()
	if requestedPath != "/api/query/canoe-cove-pe" {
		t.Errorf("requested path: got %q, want %q", requestedPath, "/api/query/canoe-cove-pe")
	}
}

// TestFetcherMapsProbabilityFromBody — proves the probability value actually
// comes from the HTTP response body, not a hard-coded zero.
func TestFetcherMapsProbabilityFromBody(t *testing.T) {
	body := []byte(`{
		"prediction": {"probability": 42, "score": 55, "category": "High", "debug": {"nextMorning": "2026-01-15T08:00:00-05:00"}},
		"city": "Springfield", "state": "IL", "region_name": "Springfield, IL"
	}`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	f := snowday.New(nil)
	f.RefreshNow(context.Background(), types.SnowDay{URL: srv.URL})

	snap := f.Snapshot()
	if snap == nil {
		t.Fatal("expected snapshot")
	}
	if snap.Probability != 42 {
		t.Errorf("Probability: got %d, want 42", snap.Probability)
	}
	if snap.Score != 55 {
		t.Errorf("Score: got %d, want 55", snap.Score)
	}
	if snap.Category != "High" {
		t.Errorf("Category: got %q, want %q", snap.Category, "High")
	}
	if snap.Location != "Springfield, IL" {
		t.Errorf("Location: got %q, want %q", snap.Location, "Springfield, IL")
	}
}

// TestFetcherRejectsNonHTTPSchemes — SSRF mitigation: file://, ftp:// etc.
// produce no snapshot rather than being fetched.
func TestFetcherRejectsNonHTTPSchemes(t *testing.T) {
	f := snowday.New(nil)
	f.RefreshNow(context.Background(), types.SnowDay{URL: "file:///etc/passwd"})
	if snap := f.Snapshot(); snap != nil {
		t.Errorf("expected no snapshot for file:// URL, got %+v", snap)
	}
}

// TestFetcherOnBrowserStyleURLWithQueryString — query strings and fragments
// from the browser URL must not break the API call.
func TestFetcherOnBrowserStyleURLWithQueryString(t *testing.T) {
	body := loadFixture(t, "canoe-cove.json")
	var mu sync.Mutex
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = r.URL.RequestURI()
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	f := snowday.New(nil)
	f.RefreshNow(context.Background(), types.SnowDay{URL: srv.URL + "/prediction/canoe-cove-pe?utm_source=x#section"})

	mu.Lock()
	defer mu.Unlock()
	if got != "/api/query/canoe-cove-pe" {
		t.Errorf("request URI: got %q, want %q", got, "/api/query/canoe-cove-pe")
	}
}

// TestFetcherOnMalformedJSONKeepsPriorSnapshot — if the body is missing the
// prediction data, treat it like a transient failure and preserve prior state.
func TestFetcherOnMalformedJSONKeepsPriorSnapshot(t *testing.T) {
	goodBody := loadFixture(t, "canoe-cove.json")
	malformed := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if malformed {
			_, _ = w.Write([]byte(`{"forecast":[]}`))
			return
		}
		_, _ = w.Write(goodBody)
	}))
	defer srv.Close()

	f := snowday.New(nil)
	cfg := types.SnowDay{URL: srv.URL}

	f.RefreshNow(context.Background(), cfg)
	first := f.Snapshot()
	if first == nil {
		t.Fatal("expected initial snapshot, got nil")
	}

	malformed = true
	f.RefreshNow(context.Background(), cfg)

	after := f.Snapshot()
	if after == nil {
		t.Fatal("expected prior snapshot preserved on malformed JSON")
	}
	if after.Category != first.Category || after.Probability != first.Probability {
		t.Errorf("snapshot replaced with garbage on malformed JSON")
	}
}

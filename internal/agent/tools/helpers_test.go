package tools_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/api/option"

	"github.com/jpatters/home-calendar/internal/agent/tools"
	"github.com/jpatters/home-calendar/internal/gcal"
	"github.com/jpatters/home-calendar/internal/types"
)

const familyCalID = "fam@group.calendar.google.com"

func familyCalendar() types.Calendar {
	return types.Calendar{ID: "1", Name: "Family", URL: "https://calendar.google.com/calendar/ical/fam%40group.calendar.google.com/private-abc/basic.ics"}
}

func nonGoogleCalendar() types.Calendar {
	return types.Calendar{ID: "2", Name: "School", URL: "https://example.com/school.ics"}
}

func halifax(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/Halifax")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}
	return loc
}

// baseDeps returns deps with config holding the Family (Google) and School
// (non-Google) calendars, interpreted in Halifax time.
func baseDeps(t *testing.T) tools.Deps {
	t.Helper()
	cfg := types.DefaultConfig()
	cfg.Calendars = []types.Calendar{familyCalendar(), nonGoogleCalendar()}
	return tools.Deps{
		Config:   func() types.Config { return cfg },
		Location: halifax(t),
	}
}

func withConfig(d tools.Deps, edit func(*types.Config)) tools.Deps {
	cfg := d.Config()
	edit(&cfg)
	d.Config = func() types.Config { return cfg }
	return d
}

// callTool finds name among tools.All(d), calls it with args and returns the
// result as the JSON the model would see.
func callTool(t *testing.T, d tools.Deps, name, args string) (string, error) {
	t.Helper()
	for _, tool := range tools.All(d) {
		if tool.Name() != name {
			continue
		}
		res, err := tool.Call(context.Background(), json.RawMessage(args))
		if err != nil {
			return "", err
		}
		b, mErr := json.Marshal(res)
		if mErr != nil {
			t.Fatalf("%s result does not marshal: %v", name, mErr)
		}
		return string(b), nil
	}
	t.Fatalf("no tool named %q", name)
	return "", nil
}

func mustCall(t *testing.T, d tools.Deps, name, args string) string {
	t.Helper()
	out, err := callTool(t, d, name, args)
	if err != nil {
		t.Fatalf("%s(%s): %v", name, args, err)
	}
	return out
}

func mustFail(t *testing.T, d tools.Deps, name, args string) error {
	t.Helper()
	out, err := callTool(t, d, name, args)
	if err == nil {
		t.Fatalf("%s(%s) = %s, want an error", name, args, out)
	}
	return err
}

type staticEvents []types.Event

func (s staticEvents) Events() []types.Event { return s }

type staticHockey struct{ snap *types.HockeySnapshot }

func (s staticHockey) Snapshot() *types.HockeySnapshot { return s.snap }

type staticBaseball struct{ snap *types.BaseballSnapshot }

func (s staticBaseball) Snapshot() *types.BaseballSnapshot { return s.snap }

type staticWeather struct{ snap *types.WeatherSnapshot }

func (s staticWeather) Snapshot() *types.WeatherSnapshot { return s.snap }

// fakeHotTub stands in for the in.touch2 module, which talks UDP and has no
// cheap fake outside its own package. It records each write.
type fakeHotTub struct {
	mu     sync.Mutex
	snap   *types.HotTubSnapshot
	writes []float64
	hosts  []string
}

func (f *fakeHotTub) Snapshot() *types.HotTubSnapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.snap
}

func (f *fakeHotTub) SetTarget(ctx context.Context, h types.HotTub, targetF float64) (*types.HotTubSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes = append(f.writes, targetF)
	f.hosts = append(f.hosts, h.Host)
	s := *f.snap
	s.TargetF = targetF
	f.snap = &s
	return &s, nil
}

// googleRequest is one request the fake Calendar API received.
type googleRequest struct {
	Method string
	Path   string
	Query  url.Values
	Body   map[string]any
}

// fakeGoogle emulates the Calendar v3 events endpoints gcal.Client uses:
// list (q + time overlap filter), get, insert, patch and delete.
type fakeGoogle struct {
	t      *testing.T
	mu     sync.Mutex
	reqs   []googleRequest
	events map[string]map[string]any // id -> stored event
}

func newGoogleClient(t *testing.T, stored ...string) (*gcal.Client, *fakeGoogle) {
	t.Helper()
	f := &fakeGoogle{t: t, events: map[string]map[string]any{}}
	for _, raw := range stored {
		var ev map[string]any
		if err := json.Unmarshal([]byte(raw), &ev); err != nil {
			t.Fatalf("bad stored event: %v", err)
		}
		f.events[ev["id"].(string)] = ev
	}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	c, err := gcal.New(context.Background(), serviceAccountJSON(t),
		option.WithEndpoint(srv.URL+"/calendar/v3/"),
		option.WithHTTPClient(srv.Client()),
	)
	if err != nil {
		t.Fatalf("gcal.New: %v", err)
	}
	return c, f
}

func (f *fakeGoogle) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	req := googleRequest{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query()}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &req.Body)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reqs = append(f.reqs, req)

	prefix := "/calendar/v3/calendars/"
	rest, ok := strings.CutPrefix(r.URL.Path, prefix)
	if !ok {
		http.NotFound(w, r)
		return
	}
	calID, tail, _ := strings.Cut(rest, "/events")
	if calID != familyCalID {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"error":{"code":404,"message":"Not Found"}}`)
		return
	}
	id := strings.TrimPrefix(tail, "/")
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodGet && id == "":
		f.writeList(w, r.URL.Query())
	case r.Method == http.MethodPost && id == "":
		req.Body["id"] = "created1"
		json.NewEncoder(w).Encode(req.Body)
	case id != "" && f.events[id] == nil:
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"error":{"code":404,"message":"Not Found"}}`)
	case r.Method == http.MethodGet:
		json.NewEncoder(w).Encode(f.events[id])
	case r.Method == http.MethodPatch:
		merged := map[string]any{}
		for k, v := range f.events[id] {
			merged[k] = v
		}
		for k, v := range req.Body {
			merged[k] = v
		}
		json.NewEncoder(w).Encode(merged)
	case r.Method == http.MethodDelete:
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeGoogle) writeList(w http.ResponseWriter, q url.Values) {
	timeMin, _ := time.Parse(time.RFC3339, q.Get("timeMin"))
	timeMax, _ := time.Parse(time.RFC3339, q.Get("timeMax"))
	edge := func(v any) time.Time {
		m, _ := v.(map[string]any)
		if s, ok := m["dateTime"].(string); ok {
			ts, _ := time.Parse(time.RFC3339, s)
			return ts
		}
		ts, _ := time.Parse("2006-01-02", m["date"].(string))
		return ts
	}
	items := []map[string]any{}
	for _, ev := range f.events {
		summary, _ := ev["summary"].(string)
		if s := q.Get("q"); s != "" && !strings.Contains(strings.ToLower(summary), strings.ToLower(s)) {
			continue
		}
		if !edge(ev["end"]).After(timeMin) || !edge(ev["start"]).Before(timeMax) {
			continue
		}
		items = append(items, ev)
	}
	json.NewEncoder(w).Encode(map[string]any{"items": items})
}

// writes returns every request other than a read.
func (f *fakeGoogle) writes() []googleRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []googleRequest
	for _, r := range f.reqs {
		if r.Method != http.MethodGet {
			out = append(out, r)
		}
	}
	return out
}

func (f *fakeGoogle) onlyWrite(t *testing.T) googleRequest {
	t.Helper()
	w := f.writes()
	if len(w) != 1 {
		t.Fatalf("Google writes = %+v, want exactly one", w)
	}
	return w[0]
}

func (f *fakeGoogle) requestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.reqs)
}

// serviceAccountJSON returns a syntactically valid service account key file.
// It is never used to authenticate: the test HTTP client bypasses OAuth.
func serviceAccountJSON(t *testing.T) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	b, _ := json.Marshal(map[string]string{
		"type":           "service_account",
		"project_id":     "test-project",
		"private_key_id": "abc123",
		"private_key":    string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
		"client_email":   "agent@test-project.iam.gserviceaccount.com",
		"client_id":      "1234567890",
		"token_uri":      "https://oauth2.googleapis.com/token",
	})
	return b
}

// sameInstant reports whether the RFC3339 value v is the instant want.
func sameInstant(t *testing.T, v any, want time.Time) bool {
	t.Helper()
	s, _ := v.(string)
	got, err := time.Parse(time.RFC3339, s)
	return err == nil && got.Equal(want)
}

func field(m map[string]any, path ...string) any {
	var cur any = m
	for _, p := range path {
		mm, _ := cur.(map[string]any)
		cur = mm[p]
	}
	return cur
}

// decode parses a tool's JSON output into T.
func decode[T any](t *testing.T, out string) T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("tool output %s does not decode: %v", out, err)
	}
	return v
}

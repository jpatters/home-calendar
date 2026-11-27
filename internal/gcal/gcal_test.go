package gcal_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/api/option"

	"github.com/jpatters/home-calendar/internal/gcal"
)

const familyCal = "family123@group.calendar.google.com"

// recorded is one request the fake Calendar API received.
type recorded struct {
	Method string
	Path   string
	Query  url.Values
	Body   map[string]any
}

// fakeCalendarAPI emulates the subset of the Google Calendar v3 REST API the
// client uses. Each route answers with a canned status and JSON body.
type fakeCalendarAPI struct {
	t      *testing.T
	mu     sync.Mutex
	reqs   []recorded
	routes map[string]cannedResponse // key: "METHOD /path"
	events []string                  // stored events served by events.list
}

type cannedResponse struct {
	status int
	body   string
}

func (f *fakeCalendarAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	rec := recorded{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query()}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &rec.Body); err != nil {
			f.t.Errorf("request body is not JSON: %v: %s", err, raw)
		}
	}
	f.mu.Lock()
	f.reqs = append(f.reqs, rec)
	resp, ok := f.routes[r.Method+" "+r.URL.Path]
	events := f.events
	f.mu.Unlock()
	if !ok && r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/events") {
		resp, ok = cannedResponse{200, f.list(events, r.URL.Query())}, true
	}
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"error":{"code":404,"message":"Not Found"}}`)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.status)
	io.WriteString(w, resp.body)
}

// list mimics events.list filtering: q matches the summary
// case-insensitively, and an event is returned when it overlaps
// [timeMin, timeMax) (end after timeMin, start before timeMax).
func (f *fakeCalendarAPI) list(events []string, q url.Values) string {
	bound := func(name string) (time.Time, bool) {
		v := q.Get(name)
		if v == "" {
			return time.Time{}, false
		}
		ts, err := time.Parse(time.RFC3339, v)
		if err != nil {
			f.t.Errorf("%s = %q is not RFC3339: %v", name, v, err)
			return time.Time{}, false
		}
		return ts, true
	}
	timeMin, hasMin := bound("timeMin")
	timeMax, hasMax := bound("timeMax")
	var out []string
	for _, raw := range events {
		var ev struct {
			Summary string
			Start   struct{ Date, DateTime string }
			End     struct{ Date, DateTime string }
		}
		if err := json.Unmarshal([]byte(raw), &ev); err != nil {
			f.t.Fatalf("bad stored event %s: %v", raw, err)
		}
		edge := func(d struct{ Date, DateTime string }) time.Time {
			if d.DateTime != "" {
				ts, _ := time.Parse(time.RFC3339, d.DateTime)
				return ts
			}
			ts, _ := time.Parse("2006-01-02", d.Date)
			return ts
		}
		if s := q.Get("q"); s != "" && !strings.Contains(strings.ToLower(ev.Summary), strings.ToLower(s)) {
			continue
		}
		if hasMin && !edge(ev.End).After(timeMin) {
			continue
		}
		if hasMax && !edge(ev.Start).Before(timeMax) {
			continue
		}
		out = append(out, raw)
	}
	return `{"items":[` + strings.Join(out, ",") + `]}`
}

func (f *fakeCalendarAPI) only(t *testing.T) recorded {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.reqs) != 1 {
		t.Fatalf("expected exactly one API request, got %d: %+v", len(f.reqs), f.reqs)
	}
	return f.reqs[0]
}

// patch returns the one PATCH request received.
func (f *fakeCalendarAPI) patch(t *testing.T) recorded {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	var patches []recorded
	for _, r := range f.reqs {
		if r.Method == http.MethodPatch {
			patches = append(patches, r)
		}
	}
	if len(patches) != 1 {
		t.Fatalf("expected exactly one PATCH, got %d: %+v", len(patches), f.reqs)
	}
	return patches[0]
}

func newClient(t *testing.T, routes map[string]cannedResponse) (*gcal.Client, *fakeCalendarAPI) {
	t.Helper()
	return newClientWithEvents(t, routes)
}

func newClientWithEvents(t *testing.T, routes map[string]cannedResponse, events ...string) (*gcal.Client, *fakeCalendarAPI) {
	t.Helper()
	api := &fakeCalendarAPI{t: t, routes: routes, events: events}
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)
	c, err := gcal.New(context.Background(), serviceAccountJSON(t),
		option.WithEndpoint(srv.URL+"/calendar/v3/"),
		option.WithHTTPClient(srv.Client()),
	)
	if err != nil {
		t.Fatalf("gcal.New: %v", err)
	}
	return c, api
}

// serviceAccountJSON returns a syntactically valid service account key file
// with a freshly generated private key. It is never used to authenticate.
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
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	b, err := json.Marshal(map[string]string{
		"type":           "service_account",
		"project_id":     "test-project",
		"private_key_id": "abc123",
		"private_key":    string(keyPEM),
		"client_email":   "agent@test-project.iam.gserviceaccount.com",
		"client_id":      "1234567890",
		"token_uri":      "https://oauth2.googleapis.com/token",
	})
	if err != nil {
		t.Fatalf("marshal credentials: %v", err)
	}
	return b
}

func eventsPath(calID string) string {
	return "/calendar/v3/calendars/" + calID + "/events"
}

func toronto(t *testing.T) *time.Location {
	t.Helper()
	return zone(t, "America/Toronto")
}

func zone(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}
	return loc
}

// field walks nested JSON objects: field(body, "start", "date").
func field(m map[string]any, keys ...string) (any, bool) {
	var cur any = m
	for _, k := range keys {
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = obj[k]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func str(m map[string]any, keys ...string) string {
	v, _ := field(m, keys...)
	s, _ := v.(string)
	return s
}

// --- CalendarIDFromICalURL ---

func TestCalendarIDFromICalURL(t *testing.T) {
	cases := []struct {
		url  string
		want string
	}{
		{"https://calendar.google.com/calendar/ical/family123%40group.calendar.google.com/private-0123456789abcdef/basic.ics", "family123@group.calendar.google.com"},
		{"https://calendar.google.com/calendar/ical/me%40gmail.com/private-deadbeef/basic.ics", "me@gmail.com"},
		{"https://calendar.google.com/calendar/ical/me%40gmail.com/public/basic.ics", "me@gmail.com"},
		{"https://calendar.google.com/calendar/ical/en.canadian%23holiday%40group.v.calendar.google.com/public/basic.ics", "en.canadian#holiday@group.v.calendar.google.com"},
	}
	for _, tc := range cases {
		got, err := gcal.CalendarIDFromICalURL(tc.url)
		if err != nil {
			t.Errorf("%s: unexpected error %v", tc.url, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.url, got, tc.want)
		}
	}
}

func TestCalendarIDFromICalURLRejectsNonGoogleURLs(t *testing.T) {
	for _, u := range []string{
		"",
		"not a url",
		"https://p01-caldav.icloud.com/published/2/abcdef",
		"https://outlook.office365.com/owa/calendar/abc/reachcalendar.ics",
		"https://calendar.google.com/calendar/embed?src=me%40gmail.com",
		"https://evil.example.com/calendar/ical/me%40gmail.com/public/basic.ics",
	} {
		if id, err := gcal.CalendarIDFromICalURL(u); err == nil {
			t.Errorf("%q: expected error, got id %q", u, id)
		}
	}
}

// --- LoadCredentials / New ---

func TestLoadCredentialsAcceptsInlineJSON(t *testing.T) {
	creds := serviceAccountJSON(t)
	got, err := gcal.LoadCredentials(string(creds))
	if err != nil {
		t.Fatalf("LoadCredentials: %v", err)
	}
	if string(got) != string(creds) {
		t.Errorf("got %q, want the inline JSON unchanged", got)
	}
}

func TestLoadCredentialsReadsFilePath(t *testing.T) {
	creds := serviceAccountJSON(t)
	path := filepath.Join(t.TempDir(), "sa.json")
	if err := os.WriteFile(path, creds, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := gcal.LoadCredentials(path)
	if err != nil {
		t.Fatalf("LoadCredentials: %v", err)
	}
	if string(got) != string(creds) {
		t.Errorf("got %q, want file contents", got)
	}
}

func TestLoadCredentialsFailsForEmptyOrMissing(t *testing.T) {
	for _, v := range []string{"", "   ", filepath.Join(t.TempDir(), "missing.json")} {
		if _, err := gcal.LoadCredentials(v); err == nil {
			t.Errorf("%q: expected error", v)
		}
	}
}

func TestNewRejectsInvalidCredentials(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	for _, creds := range []string{"", "not json", `{"type":"authorized_user"}`} {
		_, err := gcal.New(context.Background(), []byte(creds),
			option.WithEndpoint(srv.URL+"/calendar/v3/"),
			option.WithHTTPClient(srv.Client()),
		)
		if err == nil {
			t.Errorf("%q: expected error", creds)
		}
	}
}

// --- CreateEvent ---

func TestCreateEventSendsTimedEventWithTimeZone(t *testing.T) {
	loc := zone(t, "America/Halifax")
	c, api := newClient(t, map[string]cannedResponse{
		"POST " + eventsPath(familyCal): {200, `{"id":"evt1","summary":"Dentist","location":"Main St","description":"Bring card",
			"start":{"dateTime":"2026-12-01T15:00:00-04:00","timeZone":"America/Halifax"},
			"end":{"dateTime":"2026-12-01T16:00:00-04:00","timeZone":"America/Halifax"}}`},
	})

	start := time.Date(2026, 12, 1, 15, 0, 0, 0, loc)
	got, err := c.CreateEvent(context.Background(), familyCal, gcal.NewEvent{
		Title: "Dentist", Start: start, End: start.Add(time.Hour),
		Location: "Main St", Description: "Bring card",
	})
	if err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}

	req := api.only(t)
	if req.Method != http.MethodPost || req.Path != eventsPath(familyCal) {
		t.Fatalf("request = %s %s", req.Method, req.Path)
	}
	if s := str(req.Body, "summary"); s != "Dentist" {
		t.Errorf("summary = %q", s)
	}
	if s := str(req.Body, "location"); s != "Main St" {
		t.Errorf("location = %q", s)
	}
	if s := str(req.Body, "description"); s != "Bring card" {
		t.Errorf("description = %q", s)
	}
	for _, side := range []struct {
		key  string
		want time.Time
	}{{"start", start}, {"end", start.Add(time.Hour)}} {
		dt, err := time.Parse(time.RFC3339, str(req.Body, side.key, "dateTime"))
		if err != nil {
			t.Errorf("%s.dateTime not RFC3339: %v (body=%v)", side.key, err, req.Body)
		} else if !dt.Equal(side.want) {
			t.Errorf("%s.dateTime = %v, want %v", side.key, dt, side.want)
		}
		if tz := str(req.Body, side.key, "timeZone"); tz != "America/Halifax" {
			t.Errorf("%s.timeZone = %q, want America/Halifax", side.key, tz)
		}
		if _, ok := field(req.Body, side.key, "date"); ok {
			t.Errorf("%s.date must not be sent for a timed event", side.key)
		}
	}

	if got.ID != "evt1" || got.Title != "Dentist" || got.AllDay {
		t.Errorf("returned event = %+v", got)
	}
	if !got.Start.Equal(start) || !got.End.Equal(start.Add(time.Hour)) {
		t.Errorf("returned times = %v–%v", got.Start, got.End)
	}
}

func TestCreateEventOmitsTimeZoneForLocalTime(t *testing.T) {
	c, api := newClient(t, map[string]cannedResponse{
		"POST " + eventsPath(familyCal): {200, `{"id":"evt3","summary":"Call","start":{"dateTime":"2026-12-01T15:00:00Z"},"end":{"dateTime":"2026-12-01T15:30:00Z"}}`},
	})
	start := time.Date(2026, 12, 1, 15, 0, 0, 0, time.Local)

	if _, err := c.CreateEvent(context.Background(), familyCal, gcal.NewEvent{Title: "Call", Start: start, End: start.Add(30 * time.Minute)}); err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}

	req := api.only(t)
	for _, side := range []struct {
		key  string
		want time.Time
	}{{"start", start}, {"end", start.Add(30 * time.Minute)}} {
		dt, err := time.Parse(time.RFC3339, str(req.Body, side.key, "dateTime"))
		if err != nil || !dt.Equal(side.want) {
			t.Errorf("%s.dateTime = %q, want %v", side.key, str(req.Body, side.key, "dateTime"), side.want)
		}
		if v, ok := field(req.Body, side.key, "timeZone"); ok {
			t.Errorf("%s.timeZone = %v, want it omitted for time.Local", side.key, v)
		}
	}
}

func TestCreateEventSendsAllDayEventAsDates(t *testing.T) {
	loc := toronto(t)
	c, api := newClient(t, map[string]cannedResponse{
		"POST " + eventsPath(familyCal): {200, `{"id":"evt2","summary":"Christmas Eve",
			"start":{"date":"2026-12-24"},"end":{"date":"2026-12-25"}}`},
	})

	got, err := c.CreateEvent(context.Background(), familyCal, gcal.NewEvent{
		Title:  "Christmas Eve",
		Start:  time.Date(2026, 12, 24, 0, 0, 0, 0, loc),
		End:    time.Date(2026, 12, 25, 0, 0, 0, 0, loc),
		AllDay: true,
	})
	if err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}

	req := api.only(t)
	if d := str(req.Body, "start", "date"); d != "2026-12-24" {
		t.Errorf("start.date = %q, want 2026-12-24", d)
	}
	if d := str(req.Body, "end", "date"); d != "2026-12-25" {
		t.Errorf("end.date = %q, want 2026-12-25 (exclusive)", d)
	}
	for _, side := range []string{"start", "end"} {
		if _, ok := field(req.Body, side, "dateTime"); ok {
			t.Errorf("%s.dateTime must not be sent for an all-day event", side)
		}
	}

	if got.ID != "evt2" || !got.AllDay {
		t.Errorf("returned event = %+v, want all-day evt2", got)
	}
	if got.Start.Format("2006-01-02") != "2026-12-24" || got.End.Format("2006-01-02") != "2026-12-25" {
		t.Errorf("returned dates = %v–%v", got.Start, got.End)
	}
}

func TestCreateEventReturnsErrorOnAPIFailure(t *testing.T) {
	c, _ := newClient(t, map[string]cannedResponse{
		"POST " + eventsPath(familyCal): {403, `{"error":{"code":403,"message":"Forbidden"}}`},
	})
	start := time.Date(2026, 12, 1, 15, 0, 0, 0, time.UTC)
	if _, err := c.CreateEvent(context.Background(), familyCal, gcal.NewEvent{Title: "x", Start: start, End: start.Add(time.Hour)}); err == nil {
		t.Fatal("expected error")
	}
}

// --- FindEvent ---

// dec1 is the whole of 2026-12-01 in Toronto (UTC-5).
var dec1From, dec1To = time.Date(2026, 12, 1, 5, 0, 0, 0, time.UTC), time.Date(2026, 12, 2, 5, 0, 0, 0, time.UTC)

const (
	dentistAt3pm    = `{"id":"dentist","summary":"Dentist","location":"Main St","start":{"dateTime":"2026-12-01T15:00:00-05:00"},"end":{"dateTime":"2026-12-01T16:00:00-05:00"}}`
	dentistFollowUp = `{"id":"followup","summary":"Dentist follow-up","start":{"dateTime":"2026-12-01T11:00:00-05:00"},"end":{"dateTime":"2026-12-01T11:30:00-05:00"}}`
	dentistNextWeek = `{"id":"dentist2","summary":"Dentist","start":{"dateTime":"2026-12-03T15:00:00-05:00"},"end":{"dateTime":"2026-12-03T16:00:00-05:00"}}`
	pdDay           = `{"id":"pd","summary":"PD Day","start":{"date":"2026-12-04"},"end":{"date":"2026-12-05"}}`
	swimAM          = `{"id":"swim_am","summary":"Swim practice","start":{"dateTime":"2026-12-01T09:00:00-05:00"},"end":{"dateTime":"2026-12-01T10:00:00-05:00"}}`
	swimPM          = `{"id":"swim_pm","summary":"swim Practice ","start":{"dateTime":"2026-12-01T17:00:00-05:00"},"end":{"dateTime":"2026-12-01T18:00:00-05:00"}}`
)

func TestFindEventRequestsExpandedRecurringInstances(t *testing.T) {
	c, api := newClientWithEvents(t, nil, dentistAt3pm)

	if _, err := c.FindEvent(context.Background(), familyCal, "Dentist", dec1From, dec1To); err != nil {
		t.Fatalf("FindEvent: %v", err)
	}
	req := api.only(t)
	if req.Method != http.MethodGet || req.Path != eventsPath(familyCal) {
		t.Fatalf("request = %s %s", req.Method, req.Path)
	}
	if v := req.Query.Get("singleEvents"); v != "true" {
		t.Errorf("singleEvents = %q, want true", v)
	}
}

func TestFindEventSearchesOnlyTheGivenWindow(t *testing.T) {
	c, _ := newClientWithEvents(t, nil, dentistAt3pm)
	at3pm := time.Date(2026, 12, 1, 20, 0, 0, 0, time.UTC)

	if _, err := c.FindEvent(context.Background(), familyCal, "Dentist", at3pm.Add(-time.Hour), at3pm.Add(time.Hour)); err != nil {
		t.Errorf("window around 3pm: %v, want the 3pm appointment", err)
	}
	_, err := c.FindEvent(context.Background(), familyCal, "Dentist", at3pm.Add(2*time.Hour), at3pm.Add(4*time.Hour))
	if !errors.Is(err, gcal.ErrNotFound) {
		t.Errorf("window after the appointment: err = %v, want ErrNotFound", err)
	}
}

func TestFindEventReturnsCaseInsensitiveTitleMatchNearTime(t *testing.T) {
	c, _ := newClientWithEvents(t, nil, dentistFollowUp, dentistAt3pm, dentistNextWeek)

	// The 3pm appointment is on Dec 1; the one two days later is not.
	got, err := c.FindEvent(context.Background(), familyCal, "dentist", dec1From, dec1To)
	if err != nil {
		t.Fatalf("FindEvent: %v", err)
	}
	if got.ID != "dentist" || got.Title != "Dentist" || got.Location != "Main St" {
		t.Errorf("got %+v, want the Dentist event on Dec 1", got)
	}
	want := time.Date(2026, 12, 1, 20, 0, 0, 0, time.UTC)
	if !got.Start.Equal(want) {
		t.Errorf("Start = %v, want %v", got.Start, want)
	}
}

func TestFindEventReturnsAllDayEvent(t *testing.T) {
	c, _ := newClientWithEvents(t, nil, pdDay)
	got, err := c.FindEvent(context.Background(), familyCal, "PD Day", dec1From.AddDate(0, 0, 3), dec1To.AddDate(0, 0, 3))
	if err != nil {
		t.Fatalf("FindEvent: %v", err)
	}
	if got.ID != "pd" || !got.AllDay || got.Start.Format("2006-01-02") != "2026-12-04" {
		t.Errorf("got %+v, want all-day PD Day on 2026-12-04", got)
	}
}

func TestFindEventReturnsErrNotFound(t *testing.T) {
	for name, events := range map[string][]string{
		"empty calendar":       nil,
		"only a similar title": {dentistFollowUp},
		"only two days away":   {dentistNextWeek},
	} {
		t.Run(name, func(t *testing.T) {
			c, _ := newClientWithEvents(t, nil, events...)
			_, err := c.FindEvent(context.Background(), familyCal, "Dentist", dec1From, dec1To)
			if !errors.Is(err, gcal.ErrNotFound) {
				t.Errorf("err = %v, want ErrNotFound", err)
			}
		})
	}
}

func TestFindEventReturnsErrAmbiguousListingCandidates(t *testing.T) {
	c, _ := newClientWithEvents(t, nil, swimAM, swimPM)

	_, err := c.FindEvent(context.Background(), familyCal, "Swim Practice", dec1From, dec1To)
	if !errors.Is(err, gcal.ErrAmbiguous) {
		t.Fatalf("err = %v, want ErrAmbiguous", err)
	}
	msg := err.Error()
	if !strings.Contains(strings.ToLower(msg), "swim practice") {
		t.Errorf("error %q should name the candidate titles", msg)
	}
	if !strings.Contains(msg, "09:00") || !strings.Contains(msg, "17:00") {
		t.Errorf("error %q should include each candidate's start time", msg)
	}
}

// --- UpdateEvent ---

const patchedEvent = `{"id":"evt1","summary":"Dentist (moved)","location":"",
	"start":{"dateTime":"2026-12-02T10:00:00-05:00"},"end":{"dateTime":"2026-12-02T11:00:00-05:00"}}`

func TestUpdateEventSendsOnlyPatchedFields(t *testing.T) {
	c, api := newClient(t, map[string]cannedResponse{
		"PATCH " + eventsPath(familyCal) + "/evt1": {200, patchedEvent},
	})
	title := "Dentist (moved)"

	got, err := c.UpdateEvent(context.Background(), familyCal, "evt1", gcal.EventPatch{Title: &title})
	if err != nil {
		t.Fatalf("UpdateEvent: %v", err)
	}

	req := api.only(t)
	if req.Method != http.MethodPatch || req.Path != eventsPath(familyCal)+"/evt1" {
		t.Fatalf("request = %s %s", req.Method, req.Path)
	}
	if len(req.Body) != 1 || str(req.Body, "summary") != title {
		t.Errorf("patch body = %v, want only summary", req.Body)
	}
	if got.ID != "evt1" || got.Title != title {
		t.Errorf("returned event = %+v", got)
	}
}

func TestUpdateEventCanClearLocation(t *testing.T) {
	c, api := newClient(t, map[string]cannedResponse{
		"PATCH " + eventsPath(familyCal) + "/evt1": {200, patchedEvent},
	})
	empty := ""

	if _, err := c.UpdateEvent(context.Background(), familyCal, "evt1", gcal.EventPatch{Location: &empty}); err != nil {
		t.Fatalf("UpdateEvent: %v", err)
	}

	req := api.only(t)
	v, ok := field(req.Body, "location")
	if !ok || v != "" {
		t.Errorf("patch body = %v, want location sent as empty string", req.Body)
	}
}

func TestUpdateEventMovesTimedEvent(t *testing.T) {
	loc := toronto(t)
	c, api := newClient(t, map[string]cannedResponse{
		"GET " + eventsPath(familyCal) + "/evt1":   {200, dentistAt3pm},
		"PATCH " + eventsPath(familyCal) + "/evt1": {200, patchedEvent},
	})
	start := time.Date(2026, 12, 2, 10, 0, 0, 0, loc)
	end := start.Add(time.Hour)

	got, err := c.UpdateEvent(context.Background(), familyCal, "evt1", gcal.EventPatch{Start: &start, End: &end})
	if err != nil {
		t.Fatalf("UpdateEvent: %v", err)
	}

	req := api.patch(t)
	dt, err := time.Parse(time.RFC3339, str(req.Body, "start", "dateTime"))
	if err != nil || !dt.Equal(start) {
		t.Errorf("start.dateTime = %q, want %v", str(req.Body, "start", "dateTime"), start)
	}
	if tz := str(req.Body, "start", "timeZone"); tz != "America/Toronto" {
		t.Errorf("start.timeZone = %q", tz)
	}
	if _, ok := field(req.Body, "summary"); ok {
		t.Errorf("summary must not be sent when Title is nil: %v", req.Body)
	}
	// A patch merges into the stored start/end, so an all-day date must be
	// explicitly nulled for the event to become timed.
	for _, side := range []string{"start", "end"} {
		v, ok := field(req.Body, side, "date")
		if !ok || v != nil {
			t.Errorf("%s.date = %v (present=%v), want explicit null", side, v, ok)
		}
	}
	if !got.Start.Equal(start) {
		t.Errorf("returned Start = %v, want %v", got.Start, start)
	}
}

func TestUpdateEventConvertsToAllDay(t *testing.T) {
	loc := toronto(t)
	c, api := newClient(t, map[string]cannedResponse{
		"PATCH " + eventsPath(familyCal) + "/evt1": {200, `{"id":"evt1","summary":"Dentist","start":{"date":"2026-12-02"},"end":{"date":"2026-12-03"}}`},
	})
	start := time.Date(2026, 12, 2, 0, 0, 0, 0, loc)
	end := time.Date(2026, 12, 3, 0, 0, 0, 0, loc)
	allDay := true

	got, err := c.UpdateEvent(context.Background(), familyCal, "evt1", gcal.EventPatch{Start: &start, End: &end, AllDay: &allDay})
	if err != nil {
		t.Fatalf("UpdateEvent: %v", err)
	}

	req := api.only(t)
	if d := str(req.Body, "start", "date"); d != "2026-12-02" {
		t.Errorf("start.date = %q", d)
	}
	if d := str(req.Body, "end", "date"); d != "2026-12-03" {
		t.Errorf("end.date = %q", d)
	}
	// A patch merges into the stored start/end, so the old dateTime must be
	// explicitly nulled for the event to become all-day.
	for _, side := range []string{"start", "end"} {
		v, ok := field(req.Body, side, "dateTime")
		if !ok || v != nil {
			t.Errorf("%s.dateTime = %v (present=%v), want explicit null", side, v, ok)
		}
	}
	if !got.AllDay {
		t.Errorf("returned event = %+v, want all-day", got)
	}
}

func TestUpdateEventWithOnlyStartKeepsDuration(t *testing.T) {
	loc := toronto(t)
	c, api := newClient(t, map[string]cannedResponse{
		"GET " + eventsPath(familyCal) + "/evt1": {200, `{"id":"evt1","summary":"Dentist",
			"start":{"dateTime":"2026-12-01T15:00:00-05:00"},"end":{"dateTime":"2026-12-01T16:30:00-05:00"}}`},
		"PATCH " + eventsPath(familyCal) + "/evt1": {200, `{"id":"evt1","summary":"Dentist",
			"start":{"dateTime":"2026-12-02T10:00:00-05:00"},"end":{"dateTime":"2026-12-02T11:30:00-05:00"}}`},
	})
	start := time.Date(2026, 12, 2, 10, 0, 0, 0, loc)

	if _, err := c.UpdateEvent(context.Background(), familyCal, "evt1", gcal.EventPatch{Start: &start}); err != nil {
		t.Fatalf("UpdateEvent: %v", err)
	}

	patch := api.patch(t)
	end, err := time.Parse(time.RFC3339, str(patch.Body, "end", "dateTime"))
	if err != nil || !end.Equal(start.Add(90*time.Minute)) {
		t.Errorf("end.dateTime = %q, want %v (original 90 min duration)", str(patch.Body, "end", "dateTime"), start.Add(90*time.Minute))
	}
}

func TestUpdateEventMovingAllDayEventWithoutAllDayKeepsItAllDay(t *testing.T) {
	loc := toronto(t)
	c, api := newClient(t, map[string]cannedResponse{
		"GET " + eventsPath(familyCal) + "/pd":   {200, pdDay},
		"PATCH " + eventsPath(familyCal) + "/pd": {200, `{"id":"pd","summary":"PD Day","start":{"date":"2026-12-10"},"end":{"date":"2026-12-12"}}`},
	})
	start := time.Date(2026, 12, 10, 0, 0, 0, 0, loc)
	end := time.Date(2026, 12, 12, 0, 0, 0, 0, loc)

	if _, err := c.UpdateEvent(context.Background(), familyCal, "pd", gcal.EventPatch{Start: &start, End: &end}); err != nil {
		t.Fatalf("UpdateEvent: %v", err)
	}

	patch := api.patch(t)
	if str(patch.Body, "start", "date") != "2026-12-10" || str(patch.Body, "end", "date") != "2026-12-12" {
		t.Errorf("start/end = %v / %v, want dates 2026-12-10 / 2026-12-12", patch.Body["start"], patch.Body["end"])
	}
	if _, ok := field(patch.Body, "start", "dateTime"); ok && str(patch.Body, "start", "dateTime") != "" {
		t.Errorf("start = %v, want no dateTime for an all-day event", patch.Body["start"])
	}
}

func TestUpdateEventReturnsErrNotFoundForMissingOrDeletedEvent(t *testing.T) {
	title := "x"
	for _, status := range []int{404, 410} {
		c, _ := newClient(t, map[string]cannedResponse{
			"PATCH " + eventsPath(familyCal) + "/gone": {status, `{"error":{"code":` + strconv.Itoa(status) + `,"message":"gone"}}`},
		})
		_, err := c.UpdateEvent(context.Background(), familyCal, "gone", gcal.EventPatch{Title: &title})
		if !errors.Is(err, gcal.ErrNotFound) {
			t.Errorf("status %d: err = %v, want ErrNotFound", status, err)
		}
	}
}

// --- DeleteEvent ---

func TestDeleteEventDeletesByID(t *testing.T) {
	c, api := newClient(t, map[string]cannedResponse{
		"DELETE " + eventsPath(familyCal) + "/evt1": {204, ``},
	})
	if err := c.DeleteEvent(context.Background(), familyCal, "evt1"); err != nil {
		t.Fatalf("DeleteEvent: %v", err)
	}
	req := api.only(t)
	if req.Method != http.MethodDelete || req.Path != eventsPath(familyCal)+"/evt1" {
		t.Errorf("request = %s %s", req.Method, req.Path)
	}
}

func TestDeleteEventReturnsErrNotFoundForMissingOrDeletedEvent(t *testing.T) {
	for _, status := range []int{404, 410} {
		c, _ := newClient(t, map[string]cannedResponse{
			"DELETE " + eventsPath(familyCal) + "/evt1": {status, `{"error":{"code":` + strconv.Itoa(status) + `,"message":"gone"}}`},
		})
		if err := c.DeleteEvent(context.Background(), familyCal, "evt1"); !errors.Is(err, gcal.ErrNotFound) {
			t.Errorf("status %d: err = %v, want ErrNotFound", status, err)
		}
	}
}

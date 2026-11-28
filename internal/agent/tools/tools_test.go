package tools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jpatters/home-calendar/internal/agent/tools"
	"github.com/jpatters/home-calendar/internal/hottub"
	"github.com/jpatters/home-calendar/internal/pool"
	"github.com/jpatters/home-calendar/internal/types"
)

func TestAllOffersEveryToolWithAStrictSchema(t *testing.T) {
	d, _ := googleDeps(t)
	d.Memories = memoryStore(t)
	want := []string{
		"create_event", "delete_event", "forget", "get_hot_tub", "get_pool", "get_scores", "get_weather",
		"list_events", "remember", "set_hot_tub_target", "set_pool_target", "update_event", "update_memory",
	}

	var names []string
	for _, tool := range tools.All(d) {
		names = append(names, tool.Name())
		if strings.TrimSpace(tool.Description()) == "" {
			t.Errorf("%s has no description", tool.Name())
		}
		s := tool.Schema()
		if s["type"] != "object" || s["additionalProperties"] != false {
			t.Errorf("%s schema is not a closed object: %v", tool.Name(), s)
		}
		props, _ := s["properties"].(map[string]any)
		required := map[string]bool{}
		for _, r := range requiredList(s["required"]) {
			required[r] = true
		}
		for p := range props {
			if !required[p] {
				t.Errorf("%s: property %q is not required (strict mode needs every property required, nullable if optional)", tool.Name(), p)
			}
		}
		if len(required) != len(props) {
			t.Errorf("%s: required %v does not match properties", tool.Name(), s["required"])
		}
	}
	sort.Strings(names)
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("tools = %v, want %v", names, want)
	}
}

func TestAllBuildsIdenticalSchemasEveryTime(t *testing.T) {
	d, _ := googleDeps(t)
	schemas := func() string {
		var b strings.Builder
		for _, tool := range tools.All(d) {
			raw, err := json.Marshal(tool.Schema())
			if err != nil {
				t.Fatalf("%s schema does not marshal: %v", tool.Name(), err)
			}
			b.Write(raw)
		}
		return b.String()
	}

	first := schemas()
	for range 20 {
		if again := schemas(); again != first {
			t.Fatalf("schemas differ between calls:\n%s\n%s", first, again)
		}
	}
}

func requiredList(v any) []string {
	switch r := v.(type) {
	case []string:
		return r
	case []any:
		out := make([]string, len(r))
		for i, x := range r {
			out[i] = fmt.Sprint(x)
		}
		return out
	}
	return nil
}

// --- list_events ---

func eventsDeps(t *testing.T) tools.Deps {
	d := baseDeps(t)
	loc := d.Location
	d.Events = staticEvents{
		{CalendarName: "Family", Title: "Breakfast", Start: time.Date(2026, 11, 26, 8, 0, 0, 0, loc), End: time.Date(2026, 11, 26, 9, 0, 0, 0, loc)},
		{CalendarName: "Family", Title: "Dentist", Start: time.Date(2026, 11, 27, 15, 0, 0, 0, loc), End: time.Date(2026, 11, 27, 16, 0, 0, 0, loc), Location: "Main St"},
		{CalendarName: "School", Title: "Sleepover", Start: time.Date(2026, 11, 26, 20, 0, 0, 0, loc), End: time.Date(2026, 11, 27, 9, 0, 0, 0, loc)},
		{CalendarName: "School", Title: "Field trip", Start: time.Date(2026, 11, 28, 0, 0, 0, 0, loc), End: time.Date(2026, 11, 29, 0, 0, 0, 0, loc), AllDay: true},
	}
	return d
}

type listedEvent struct {
	Calendar string `json:"calendar"`
	Title    string `json:"title"`
	Start    string `json:"start"`
	End      string `json:"end"`
	AllDay   bool   `json:"allDay"`
	Location string `json:"location"`
}

func listEvents(t *testing.T, d tools.Deps, args string) []listedEvent {
	t.Helper()
	out := mustCall(t, d, "list_events", args)
	var body struct {
		Events []listedEvent `json:"events"`
	}
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatalf("list_events output %s: %v", out, err)
	}
	return body.Events
}

func titles(evs []listedEvent) string {
	var out []string
	for _, e := range evs {
		out = append(out, e.Title)
	}
	return strings.Join(out, ",")
}

func TestListEventsReturnsEventsOverlappingTheRange(t *testing.T) {
	evs := listEvents(t, eventsDeps(t), `{"start":"2026-11-27T00:00","end":"2026-11-28T00:00","calendar":null}`)

	if got := titles(evs); got != "Sleepover,Dentist" {
		t.Errorf("titles = %q, want the overnight event then the dentist, in start order", got)
	}
}

func TestListEventsAcceptsDateOnlyAndRFC3339Bounds(t *testing.T) {
	d := eventsDeps(t)

	dateOnly := listEvents(t, d, `{"start":"2026-11-28","end":"2026-11-28","calendar":null}`)
	rfc := listEvents(t, d, `{"start":"2026-11-28T03:00:00Z","end":"2026-11-29T03:00:00Z","calendar":null}`)

	if got := titles(dateOnly); got != "Field trip" {
		t.Errorf("date-only range titles = %q, want Field trip", got)
	}
	if got := titles(rfc); got != "Field trip" {
		t.Errorf("RFC3339 range titles = %q, want Field trip", got)
	}
}

func TestListEventsTreatsDateOnlyEndAsInclusiveDay(t *testing.T) {
	evs := listEvents(t, eventsDeps(t), `{"start":"2026-11-27","end":"2026-11-27","calendar":null}`)

	if got := titles(evs); got != "Sleepover,Dentist" {
		t.Errorf("titles = %q, want everything overlapping Nov 27 and nothing from the 28th", got)
	}
}

func TestListEventsFiltersByCalendarNameCaseInsensitively(t *testing.T) {
	evs := listEvents(t, eventsDeps(t), `{"start":"2026-11-26T00:00","end":"2026-11-30T00:00","calendar":"family"}`)

	if got := titles(evs); got != "Breakfast,Dentist" {
		t.Errorf("titles = %q, want only Family events", got)
	}
}

func TestListEventsReportsTimesInLocalZoneWithDetails(t *testing.T) {
	evs := listEvents(t, eventsDeps(t), `{"start":"2026-11-27T12:00","end":"2026-11-27T18:00","calendar":"Family"}`)

	if len(evs) != 1 {
		t.Fatalf("events = %+v, want the dentist", evs)
	}
	e := evs[0]
	if e.Start != "2026-11-27T15:00:00-04:00" || e.End != "2026-11-27T16:00:00-04:00" {
		t.Errorf("times = %s..%s, want Halifax local RFC3339", e.Start, e.End)
	}
	if e.Calendar != "Family" || e.Location != "Main St" || e.AllDay {
		t.Errorf("event = %+v, want calendar, location and timed", e)
	}
}

func TestListEventsRejectsUnknownCalendar(t *testing.T) {
	err := mustFail(t, eventsDeps(t), "list_events", `{"start":"2026-11-27","end":"2026-11-28","calendar":"Gym"}`)

	if !strings.Contains(err.Error(), "Family") || !strings.Contains(err.Error(), "School") {
		t.Errorf("error %q should name the calendars that exist", err)
	}
}

func TestListEventsRejectsUnparseableTime(t *testing.T) {
	mustFail(t, eventsDeps(t), "list_events", `{"start":"next tuesday","end":"2026-11-28","calendar":null}`)
}

// --- create_event ---

func googleDeps(t *testing.T, stored ...string) (tools.Deps, *fakeGoogle) {
	t.Helper()
	d := baseDeps(t)
	c, f := newGoogleClient(t, stored...)
	d.Calendar = c
	return d, f
}

func TestCreateEventInsertsTimedEventOnNamedGoogleCalendar(t *testing.T) {
	d, google := googleDeps(t)

	out := mustCall(t, d, "create_event", `{"calendar":"family","title":"Dentist","start":"2026-11-27T15:00","end":"2026-11-27T16:00","all_day":null,"location":"Main St"}`)

	w := google.onlyWrite(t)
	if w.Method != http.MethodPost || w.Path != "/calendar/v3/calendars/"+familyCalID+"/events" {
		t.Fatalf("request = %s %s, want insert on the Family calendar", w.Method, w.Path)
	}
	loc := d.Location
	if w.Body["summary"] != "Dentist" || w.Body["location"] != "Main St" {
		t.Errorf("body = %v, want title and location", w.Body)
	}
	if !sameInstant(t, field(w.Body, "start", "dateTime"), time.Date(2026, 11, 27, 15, 0, 0, 0, loc)) ||
		!sameInstant(t, field(w.Body, "end", "dateTime"), time.Date(2026, 11, 27, 16, 0, 0, 0, loc)) {
		t.Errorf("start/end = %v / %v, want 15:00–16:00 Halifax", w.Body["start"], w.Body["end"])
	}
	if field(w.Body, "start", "timeZone") != "America/Halifax" {
		t.Errorf("start = %v, want the local time zone attached", w.Body["start"])
	}
	created := decode[struct{ Event listedEvent }](t, out).Event
	if created.Title != "Dentist" || created.Calendar != "Family" || created.Start != "2026-11-27T15:00:00-04:00" {
		t.Errorf("output event = %+v, want the created event in local time", created)
	}
}

func TestCreateEventConvertsRFC3339TimesToLocalZone(t *testing.T) {
	d, google := googleDeps(t)

	mustCall(t, d, "create_event", `{"calendar":"Family","title":"Call","start":"2026-11-27T19:00:00Z","end":"2026-11-27T19:30:00Z","all_day":null,"location":null}`)

	w := google.onlyWrite(t)
	if field(w.Body, "start", "dateTime") != "2026-11-27T15:00:00-04:00" {
		t.Errorf("start = %v, want the instant expressed in Halifax time", w.Body["start"])
	}
}

func TestCreateEventAllDayTreatsEndAsInclusiveLastDay(t *testing.T) {
	cases := []struct {
		name, end, wantEnd string
	}{
		{"single day without end", "null", "2026-11-28"},
		{"two days", `"2026-11-28"`, "2026-11-29"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, google := googleDeps(t)

			mustCall(t, d, "create_event", `{"calendar":"Family","title":"Trip","start":"2026-11-27","end":`+tc.end+`,"all_day":true,"location":null}`)

			w := google.onlyWrite(t)
			if field(w.Body, "start", "date") != "2026-11-27" || field(w.Body, "end", "date") != tc.wantEnd {
				t.Errorf("start/end = %v / %v, want 2026-11-27 / %s (exclusive)", w.Body["start"], w.Body["end"], tc.wantEnd)
			}
		})
	}
}

func TestCreateEventRequiresEndForTimedEvent(t *testing.T) {
	d, google := googleDeps(t)

	mustFail(t, d, "create_event", `{"calendar":"Family","title":"Dentist","start":"2026-11-27T15:00","end":null,"all_day":null,"location":null}`)

	if n := google.requestCount(); n != 0 {
		t.Errorf("Google requests = %d, want none", n)
	}
}

func TestCalendarWriteToolsAreNotOfferedWithoutGoogleCredentials(t *testing.T) {
	d := baseDeps(t)

	offered := map[string]bool{}
	for _, tool := range tools.All(d) {
		offered[tool.Name()] = true
	}

	for _, name := range []string{"create_event", "update_event", "delete_event"} {
		if offered[name] {
			t.Errorf("%s is offered without Google credentials", name)
		}
	}
	if !offered["list_events"] {
		t.Errorf("list_events should still be offered")
	}
}

func TestCalendarWritesRejectNonGoogleCalendar(t *testing.T) {
	d, google := googleDeps(t)
	for name, args := range writeArgs("School") {
		t.Run(name, func(t *testing.T) {
			err := mustFail(t, d, name, args)
			if !strings.Contains(err.Error(), "School") {
				t.Errorf("error %q should name the calendar that can't be changed", err)
			}
		})
	}
	if n := google.requestCount(); n != 0 {
		t.Errorf("Google requests = %d, want none", n)
	}
}

func TestCalendarWritesRejectUnknownCalendar(t *testing.T) {
	d, google := googleDeps(t)
	for name, args := range writeArgs("Gym") {
		t.Run(name, func(t *testing.T) {
			err := mustFail(t, d, name, args)
			if !strings.Contains(err.Error(), "Family") {
				t.Errorf("error %q should name the calendars that exist", err)
			}
		})
	}
	if n := google.requestCount(); n != 0 {
		t.Errorf("Google requests = %d, want none", n)
	}
}

func writeArgs(calendar string) map[string]string {
	return map[string]string{
		"create_event": fmt.Sprintf(`{"calendar":%q,"title":"Dentist","start":"2026-11-27T15:00","end":"2026-11-27T16:00","all_day":null,"location":null}`, calendar),
		"update_event": fmt.Sprintf(`{"calendar":%q,"title":"Dentist","original_start":"2026-11-27T15:00","new_start":"2026-11-27T16:00","new_end":null,"new_title":null,"location":null,"all_day":null}`, calendar),
		"delete_event": fmt.Sprintf(`{"calendar":%q,"title":"Dentist","start":"2026-11-27T15:00"}`, calendar),
	}
}

// --- update_event / delete_event ---

const storedDentist = `{"id":"ev1","summary":"Dentist","start":{"dateTime":"2026-11-27T15:00:00-04:00"},"end":{"dateTime":"2026-11-27T16:00:00-04:00"}}`
const storedDentistLate = `{"id":"ev2","summary":"Dentist","start":{"dateTime":"2026-11-27T19:00:00-04:00"},"end":{"dateTime":"2026-11-27T20:00:00-04:00"}}`

func TestUpdateEventPatchesTheMatchedEvent(t *testing.T) {
	d, google := googleDeps(t, storedDentist)

	out := mustCall(t, d, "update_event", `{"calendar":"Family","title":"dentist","original_start":"2026-11-27T15:00","new_start":"2026-11-28T10:00","new_end":"2026-11-28T11:00","new_title":"Dentist (moved)","location":"Clinic","all_day":null}`)

	w := google.onlyWrite(t)
	if w.Method != http.MethodPatch || w.Path != "/calendar/v3/calendars/"+familyCalID+"/events/ev1" {
		t.Fatalf("request = %s %s, want a patch of ev1 on Family", w.Method, w.Path)
	}
	loc := d.Location
	if w.Body["summary"] != "Dentist (moved)" || w.Body["location"] != "Clinic" {
		t.Errorf("body = %v, want new title and location", w.Body)
	}
	if !sameInstant(t, field(w.Body, "start", "dateTime"), time.Date(2026, 11, 28, 10, 0, 0, 0, loc)) ||
		!sameInstant(t, field(w.Body, "end", "dateTime"), time.Date(2026, 11, 28, 11, 0, 0, 0, loc)) {
		t.Errorf("start/end = %v / %v, want Nov 28 10:00–11:00 Halifax", w.Body["start"], w.Body["end"])
	}
	updated := decode[struct{ Event listedEvent }](t, out).Event
	if updated.Title != "Dentist (moved)" || updated.Start != "2026-11-28T10:00:00-04:00" {
		t.Errorf("output event = %+v, want the updated event in local time", updated)
	}
}

func TestUpdateEventMovingAllDayEventKeepsItAllDayWithInclusiveEnd(t *testing.T) {
	d, google := googleDeps(t, `{"id":"pd","summary":"PD Day","start":{"date":"2026-11-27"},"end":{"date":"2026-11-28"}}`)

	mustCall(t, d, "update_event", `{"calendar":"Family","title":"PD Day","original_start":"2026-11-27","new_start":"2026-12-03","new_end":"2026-12-04","new_title":null,"location":null,"all_day":null}`)

	w := google.onlyWrite(t)
	if field(w.Body, "start", "date") != "2026-12-03" || field(w.Body, "end", "date") != "2026-12-05" {
		t.Errorf("start/end = %v / %v, want all-day 2026-12-03 to 2026-12-05 (exclusive)", w.Body["start"], w.Body["end"])
	}
}

func TestUpdateEventReportsAmbiguousMatchWithoutWriting(t *testing.T) {
	d, google := googleDeps(t, storedDentist, storedDentistLate)

	err := mustFail(t, d, "update_event", `{"calendar":"Family","title":"Dentist","original_start":"2026-11-27","new_start":null,"new_end":null,"new_title":"Orthodontist","location":null,"all_day":null}`)

	if !strings.Contains(err.Error(), "15:00") || !strings.Contains(err.Error(), "19:00") {
		t.Errorf("error %q should list both candidates so Ziggy can ask which", err)
	}
	if w := google.writes(); len(w) != 0 {
		t.Errorf("writes = %+v, want none", w)
	}
}

func TestUpdateEventReportsNotFound(t *testing.T) {
	d, google := googleDeps(t, storedDentist)

	err := mustFail(t, d, "update_event", `{"calendar":"Family","title":"Haircut","original_start":"2026-11-27T15:00","new_start":null,"new_end":null,"new_title":"Trim","location":null,"all_day":null}`)

	if !strings.Contains(err.Error(), "Haircut") {
		t.Errorf("error %q should name the event that wasn't found", err)
	}
	if w := google.writes(); len(w) != 0 {
		t.Errorf("writes = %+v, want none", w)
	}
}

func TestDeleteEventDeletesTheMatchedEvent(t *testing.T) {
	d, google := googleDeps(t, storedDentist)

	mustCall(t, d, "delete_event", `{"calendar":"Family","title":"Dentist","start":"2026-11-27T15:00"}`)

	w := google.onlyWrite(t)
	if w.Method != http.MethodDelete || w.Path != "/calendar/v3/calendars/"+familyCalID+"/events/ev1" {
		t.Errorf("request = %s %s, want delete of ev1 on Family", w.Method, w.Path)
	}
}

// practiceInstances are two instances of a practice recurring daily at 18:00.
var practiceInstances = []string{
	`{"id":"practice_20261126","recurringEventId":"practice","summary":"Practice","start":{"dateTime":"2026-11-26T18:00:00-04:00"},"end":{"dateTime":"2026-11-26T19:00:00-04:00"}}`,
	`{"id":"practice_20261127","recurringEventId":"practice","summary":"Practice","start":{"dateTime":"2026-11-27T18:00:00-04:00"},"end":{"dateTime":"2026-11-27T19:00:00-04:00"}}`,
}

func TestDeleteEventByDateFindsThatDaysInstance(t *testing.T) {
	d, google := googleDeps(t, practiceInstances...)

	mustCall(t, d, "delete_event", `{"calendar":"Family","title":"Practice","start":"2026-11-27"}`)

	w := google.onlyWrite(t)
	if w.Method != http.MethodDelete || !strings.HasSuffix(w.Path, "/events/practice_20261127") {
		t.Errorf("request = %s %s, want delete of the 27th's instance", w.Method, w.Path)
	}
}

func TestDeleteEventByTimeDoesNotMatchTheNeighbouringDay(t *testing.T) {
	d, google := googleDeps(t, practiceInstances...)

	mustFail(t, d, "delete_event", `{"calendar":"Family","title":"Practice","start":"2026-11-27T06:00"}`)

	if w := google.writes(); len(w) != 0 {
		t.Errorf("writes = %+v, want none", w)
	}
}

func TestDeleteEventReportsNotFoundWithoutDeleting(t *testing.T) {
	d, google := googleDeps(t, storedDentist)

	mustFail(t, d, "delete_event", `{"calendar":"Family","title":"Dentist","start":"2026-11-30T15:00"}`)

	if w := google.writes(); len(w) != 0 {
		t.Errorf("writes = %+v, want none", w)
	}
}

// --- get_scores / get_weather ---

func TestGetScoresReportsTheConfiguredTeamsGames(t *testing.T) {
	d := baseDeps(t)
	d.Hockey = staticHockey{&types.HockeySnapshot{TeamName: "Toronto Maple Leafs", LatestGame: &types.HockeyGame{Opponent: "Montréal Canadiens", TeamScore: 4, OpponentScore: 2, IsFinal: true}}}
	d.Baseball = staticBaseball{&types.BaseballSnapshot{TeamName: "Toronto Blue Jays", NextGame: &types.BaseballGame{Opponent: "New York Yankees", GameTime: time.Date(2027, 4, 1, 23, 7, 0, 0, time.UTC)}}}

	hockey := mustCall(t, d, "get_scores", `{"sport":"hockey"}`)
	baseball := mustCall(t, d, "get_scores", `{"sport":"baseball"}`)

	h := decode[types.HockeySnapshot](t, hockey)
	if h.TeamName != "Toronto Maple Leafs" || h.LatestGame == nil || h.LatestGame.Opponent != "Montréal Canadiens" || h.LatestGame.TeamScore != 4 {
		t.Errorf("hockey output %s should carry the team and last game", hockey)
	}
	b := decode[types.BaseballSnapshot](t, baseball)
	if b.TeamName != "Toronto Blue Jays" || b.NextGame == nil || b.NextGame.Opponent != "New York Yankees" {
		t.Errorf("baseball output %s should carry the baseball team's games", baseball)
	}
}

func TestGetScoresExplainsWhenSportIsNotSetUp(t *testing.T) {
	d := baseDeps(t)
	d.Hockey = staticHockey{nil}
	d.Baseball = staticBaseball{nil}

	for _, sport := range []string{"hockey", "baseball"} {
		err := mustFail(t, d, "get_scores", `{"sport":"`+sport+`"}`)
		if !strings.Contains(err.Error(), sport) {
			t.Errorf("error %q should say %s isn't set up", err, sport)
		}
	}
}

func TestGetWeatherReturnsConditionsAndForecast(t *testing.T) {
	d := baseDeps(t)
	d.Weather = staticWeather{&types.WeatherSnapshot{Units: "metric", Current: types.WeatherCurrent{TemperatureC: -3.5}, Daily: []types.WeatherDaily{{Date: "2026-11-27", MaxC: 2, MinC: -6}}}}

	out := mustCall(t, d, "get_weather", `{}`)

	w := decode[types.WeatherSnapshot](t, out)
	if w.Current.TemperatureC != -3.5 || len(w.Daily) != 1 || w.Daily[0].Date != "2026-11-27" || w.Units != "metric" {
		t.Errorf("output %s should carry current temperature and the daily forecast", out)
	}
}

func TestGetWeatherExplainsWhenNotSetUp(t *testing.T) {
	d := baseDeps(t)
	d.Weather = staticWeather{nil}

	err := mustFail(t, d, "get_weather", `{}`)

	if !strings.Contains(strings.ToLower(err.Error()), "weather") {
		t.Errorf("error %q should say weather isn't available", err)
	}
}

// --- hot tub ---

func hotTubDeps(t *testing.T, enabled bool) (tools.Deps, *fakeHotTub) {
	t.Helper()
	tub := &fakeHotTub{snap: &types.HotTubSnapshot{TemperatureF: 101, TargetF: 102, MinTargetF: 59, MaxTargetF: 104, Heating: true}}
	d := withConfig(baseDeps(t), func(c *types.Config) { c.HotTub = types.HotTub{Enabled: enabled, Host: "10.0.0.5"} })
	d.HotTub = tub
	return d, tub
}

func TestGetHotTubReportsTemperatureAndTarget(t *testing.T) {
	d, _ := hotTubDeps(t, true)

	out := mustCall(t, d, "get_hot_tub", `{}`)

	if s := decode[types.HotTubSnapshot](t, out); s.TemperatureF != 101 || s.TargetF != 102 {
		t.Errorf("output %s should carry water temperature and target", out)
	}
}

func TestHotTubToolsRefuseWhenDisabled(t *testing.T) {
	d, tub := hotTubDeps(t, false)

	mustFail(t, d, "get_hot_tub", `{}`)
	mustFail(t, d, "set_hot_tub_target", `{"target_f":100}`)

	if len(tub.writes) != 0 {
		t.Errorf("writes = %v, want none", tub.writes)
	}
}

func TestGetHotTubExplainsWhenNotReporting(t *testing.T) {
	d, tub := hotTubDeps(t, true)
	tub.snap = nil

	mustFail(t, d, "get_hot_tub", `{}`)
}

func TestSetHotTubTargetWritesToTheConfiguredTub(t *testing.T) {
	d, tub := hotTubDeps(t, true)

	out := mustCall(t, d, "set_hot_tub_target", `{"target_f":100}`)

	if len(tub.writes) != 1 || tub.writes[0] != 100 || tub.hosts[0] != "10.0.0.5" {
		t.Errorf("writes = %v to %v, want 100 to 10.0.0.5", tub.writes, tub.hosts)
	}
	if s := decode[types.HotTubSnapshot](t, out); s.TargetF != 100 {
		t.Errorf("output %s should report the new target", out)
	}
}

func TestSetHotTubTargetRefusesTargetOutsideTheTubsLimits(t *testing.T) {
	d, tub := hotTubDeps(t, true)

	err := mustFail(t, d, "set_hot_tub_target", `{"target_f":120}`)

	if !strings.Contains(err.Error(), "104") {
		t.Errorf("error %q should state the tub's limit", err)
	}
	if len(tub.writes) != 0 {
		t.Errorf("writes = %v, want none", tub.writes)
	}
}

func TestSetHotTubTargetSurfacesDriverRejection(t *testing.T) {
	d := withConfig(baseDeps(t), func(c *types.Config) { c.HotTub = types.HotTub{Enabled: true, Host: "127.0.0.1:1"} })
	d.HotTub = hottub.New(nil)

	err := mustFail(t, d, "set_hot_tub_target", `{"target_f":100.5}`)

	if !strings.Contains(err.Error(), "whole degree") {
		t.Errorf("error %q should pass the driver's reason through", err)
	}
}

// --- pool ---

// thermostat emulates the ESPHome "Pool Heater" climate entity with 15–35 °C
// limits.
type thermostat struct {
	mu     sync.Mutex
	target float64
	sets   []string
}

func (d *thermostat) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	defer d.mu.Unlock()
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/climate/Pool Heater":
		limits := ""
		if r.URL.Query().Get("detail") == "all" {
			limits = `,"min_temp":"15.0","max_temp":"35.0","step":0.5`
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"climate/Pool Heater","mode":"HEAT","action":"HEATING","state":"HEAT","current_temperature":"25.2","target_temperature":"%.1f"%s}`, d.target, limits)
	case r.Method == http.MethodPost && r.URL.Path == "/climate/Pool Heater/set":
		d.sets = append(d.sets, r.URL.RawQuery)
		v, err := strconv.ParseFloat(r.URL.Query().Get("target_temperature"), 64)
		if err != nil {
			http.Error(w, "bad", http.StatusBadRequest)
			return
		}
		d.target = v
	default:
		http.NotFound(w, r)
	}
}

func (d *thermostat) setRequests() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.sets...)
}

func poolDeps(t *testing.T, enabled bool) (tools.Deps, *thermostat, *pool.Fetcher) {
	t.Helper()
	dev := &thermostat{target: 27}
	srv := httptest.NewServer(dev)
	t.Cleanup(srv.Close)
	d := withConfig(baseDeps(t), func(c *types.Config) { c.Pool = types.Pool{Enabled: enabled, DeviceURL: srv.URL} })
	f := pool.New(nil)
	d.Pool = f
	return d, dev, f
}

func TestGetPoolReportsTemperatureAndTarget(t *testing.T) {
	d, _, f := poolDeps(t, true)
	f.RefreshNow(context.Background(), d.Config().Pool)

	out := mustCall(t, d, "get_pool", `{}`)

	if s := decode[types.PoolSnapshot](t, out); s.TemperatureC != 25.2 || s.TargetC != 27 {
		t.Errorf("output %s should carry water temperature and target", out)
	}
}

func TestSetPoolTargetWritesToTheHeater(t *testing.T) {
	d, dev, _ := poolDeps(t, true)

	out := mustCall(t, d, "set_pool_target", `{"target_c":29}`)

	if sets := dev.setRequests(); len(sets) != 1 || !strings.Contains(sets[0], "target_temperature=29") {
		t.Errorf("set requests = %v, want one with target 29", sets)
	}
	if s := decode[types.PoolSnapshot](t, out); s.TargetC != 29 {
		t.Errorf("output %s should report the new target", out)
	}
}

func TestSetPoolTargetRejectsOutOfRangeTarget(t *testing.T) {
	d, dev, _ := poolDeps(t, true)

	err := mustFail(t, d, "set_pool_target", `{"target_c":50}`)

	if !strings.Contains(err.Error(), "35") {
		t.Errorf("error %q should state the heater's limit", err)
	}
	if sets := dev.setRequests(); len(sets) != 0 {
		t.Errorf("set requests = %v, want none", sets)
	}
}

func TestPoolToolsRefuseWhenDisabled(t *testing.T) {
	d, dev, _ := poolDeps(t, false)

	mustFail(t, d, "get_pool", `{}`)
	mustFail(t, d, "set_pool_target", `{"target_c":28}`)

	if sets := dev.setRequests(); len(sets) != 0 {
		t.Errorf("set requests = %v, want none", sets)
	}
}

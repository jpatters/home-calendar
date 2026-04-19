package ical

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	ics "github.com/arran4/golang-ical"

	"github.com/jpatters/home-calendar/internal/types"
)

const allDaySingleICS = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//test//test//EN
BEGIN:VEVENT
UID:all-day-1@test
SUMMARY:All Day Event
DTSTART;VALUE=DATE:20260419
DTEND;VALUE=DATE:20260420
END:VEVENT
END:VCALENDAR
`

const allDayMultiICS = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//test//test//EN
BEGIN:VEVENT
UID:all-day-multi@test
SUMMARY:Three Day Trip
DTSTART;VALUE=DATE:20260419
DTEND;VALUE=DATE:20260422
END:VEVENT
END:VCALENDAR
`

const timedICS = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//test//test//EN
BEGIN:VEVENT
UID:timed-1@test
SUMMARY:Meeting
DTSTART:20260419T150000Z
DTEND:20260419T160000Z
END:VEVENT
END:VCALENDAR
`

func marshalFirstEvent(t *testing.T, icsBody string) map[string]any {
	t.Helper()
	cal, err := ics.ParseCalendar(strings.NewReader(icsBody))
	if err != nil {
		t.Fatalf("parse ICS: %v", err)
	}
	windowStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	windowEnd := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	evs := expandCalendar(cal, types.Calendar{ID: "c1", Name: "Cal", Color: "#fff"}, windowStart, windowEnd)
	if len(evs) == 0 {
		t.Fatal("no events returned from parser")
	}
	raw, err := json.Marshal(evs[0])
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	return out
}

// withLocal temporarily swaps time.Local. Not safe under t.Parallel —
// do not add parallel tests to this file.
func withLocal(t *testing.T, loc *time.Location, fn func()) {
	t.Helper()
	orig := time.Local
	time.Local = loc
	defer func() { time.Local = orig }()
	fn()
}

func mustLoadLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("location %s unavailable: %v", name, err)
	}
	return loc
}

func TestAllDayEventSerializesAsDateOnly(t *testing.T) {
	cases := []struct {
		name string
		loc  *time.Location
	}{
		{"UTC", time.UTC},
		{"America/Los_Angeles", mustLoadLocation(t, "America/Los_Angeles")},
		{"Europe/Berlin", mustLoadLocation(t, "Europe/Berlin")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withLocal(t, tc.loc, func() {
				got := marshalFirstEvent(t, allDaySingleICS)
				if got["allDay"] != true {
					t.Errorf("allDay: want true, got %v", got["allDay"])
				}
				if got["start"] != "2026-04-19" {
					t.Errorf("start: want 2026-04-19, got %v", got["start"])
				}
				if got["end"] != "2026-04-20" {
					t.Errorf("end: want 2026-04-20, got %v", got["end"])
				}
			})
		})
	}
}

func TestTimedEventSerializesAsRFC3339(t *testing.T) {
	got := marshalFirstEvent(t, timedICS)
	if got["allDay"] != false {
		t.Errorf("allDay: want false, got %v", got["allDay"])
	}
	startStr, ok := got["start"].(string)
	if !ok {
		t.Fatalf("start: want string, got %T", got["start"])
	}
	parsed, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		t.Fatalf("start %q not RFC3339: %v", startStr, err)
	}
	want := time.Date(2026, 4, 19, 15, 0, 0, 0, time.UTC)
	if !parsed.Equal(want) {
		t.Errorf("start: want %v, got %v", want, parsed)
	}
}

func TestMultiDayAllDayEndIsExclusive(t *testing.T) {
	got := marshalFirstEvent(t, allDayMultiICS)
	if got["allDay"] != true {
		t.Errorf("allDay: want true, got %v", got["allDay"])
	}
	if got["start"] != "2026-04-19" {
		t.Errorf("start: want 2026-04-19, got %v", got["start"])
	}
	if got["end"] != "2026-04-22" {
		t.Errorf("end: want 2026-04-22 (exclusive), got %v", got["end"])
	}
}

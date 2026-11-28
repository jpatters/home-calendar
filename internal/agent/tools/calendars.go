package tools

import (
	"fmt"
	"strings"
	"time"

	"github.com/jpatters/home-calendar/internal/gcal"
	"github.com/jpatters/home-calendar/internal/types"
)

// eventView is how an event is shown to the model. Timed events use RFC3339
// in the local zone; all-day events use dates with End as the last day (not
// the exclusive day after), matching how the tools accept them.
type eventView struct {
	Calendar string `json:"calendar"`
	Title    string `json:"title"`
	Start    string `json:"start"`
	End      string `json:"end"`
	AllDay   bool   `json:"allDay"`
	Location string `json:"location,omitempty"`
}

func viewEvent(calendar, title string, start, end time.Time, allDay bool, location string, loc *time.Location) eventView {
	v := eventView{Calendar: calendar, Title: title, AllDay: allDay, Location: location}
	if allDay {
		v.Start = start.Format(dateLayout)
		v.End = end.AddDate(0, 0, -1).Format(dateLayout)
	} else {
		v.Start = start.In(loc).Format(time.RFC3339)
		v.End = end.In(loc).Format(time.RFC3339)
	}
	return v
}

func viewGoogleEvent(calendar string, e gcal.Event, loc *time.Location) eventView {
	return viewEvent(calendar, e.Title, e.Start, e.End, e.AllDay, e.Location, loc)
}

// findCalendar returns the configured calendar called name, ignoring case.
func findCalendar(cfg types.Config, name string) (types.Calendar, error) {
	want := strings.TrimSpace(name)
	names := make([]string, len(cfg.Calendars))
	for i, c := range cfg.Calendars {
		if strings.EqualFold(strings.TrimSpace(c.Name), want) {
			return c, nil
		}
		names[i] = c.Name
	}
	if len(names) == 0 {
		return types.Calendar{}, fmt.Errorf("there is no calendar called %q; no calendars are configured", name)
	}
	return types.Calendar{}, fmt.Errorf("there is no calendar called %q; the calendars are %s", name, strings.Join(names, ", "))
}

// googleCalendar resolves name to a configured calendar and its Google
// Calendar id.
func googleCalendar(cfg types.Config, name string) (types.Calendar, string, error) {
	cal, err := findCalendar(cfg, name)
	if err != nil {
		return types.Calendar{}, "", err
	}
	id, err := gcal.CalendarIDFromICalURL(cal.URL)
	if err != nil {
		return types.Calendar{}, "", fmt.Errorf("the %s calendar isn't a Google calendar, so it can't be changed from here", cal.Name)
	}
	return cal, id, nil
}

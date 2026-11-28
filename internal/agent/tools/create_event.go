package tools

import (
	"context"
	"errors"

	"github.com/jpatters/home-calendar/internal/agent"
	"github.com/jpatters/home-calendar/internal/gcal"
)

type createEventArgs struct {
	Calendar string  `json:"calendar"`
	Title    string  `json:"title"`
	Start    string  `json:"start"`
	End      *string `json:"end"`
	AllDay   *bool   `json:"all_day"`
	Location *string `json:"location"`
}

func createEvent(d Deps) agent.Tool {
	return tool[createEventArgs]{
		name:        "create_event",
		description: "Add an event to a Google calendar.",
		schema: object(map[string]any{
			"calendar": prop("string", "Calendar name."),
			"title":    prop("string", "Event title."),
			"start":    prop("string", "When it starts; a date for all-day events. "+timeFormatHelp),
			"end":      nullable("string", "When it ends. Required for timed events. For all-day events the last day (inclusive), or null for a single day. "+timeFormatHelp),
			"all_day":  nullable("boolean", "True for an all-day event."),
			"location": nullable("string", "Where it is, or null."),
		}),
		fn: func(ctx context.Context, a createEventArgs) (any, error) {
			cal, calID, err := googleCalendar(d.Config(), a.Calendar)
			if err != nil {
				return nil, err
			}
			ev := gcal.NewEvent{Title: a.Title, AllDay: a.AllDay != nil && *a.AllDay}
			if a.Location != nil {
				ev.Location = *a.Location
			}
			if ev.Start, _, err = parseTime(a.Start, d.Location); err != nil {
				return nil, err
			}
			switch {
			case ev.AllDay && a.End == nil:
				ev.End = ev.Start.AddDate(0, 0, 1)
			case ev.AllDay:
				end, _, err := parseTime(*a.End, d.Location)
				if err != nil {
					return nil, err
				}
				ev.End = end.AddDate(0, 0, 1)
			case a.End == nil:
				return nil, errors.New("a timed event needs an end time; ask how long it is, or make it all day")
			default:
				if ev.End, _, err = parseTime(*a.End, d.Location); err != nil {
					return nil, err
				}
			}
			created, err := d.Calendar.CreateEvent(ctx, calID, ev)
			if err != nil {
				return nil, err
			}
			return map[string]any{"event": viewGoogleEvent(cal.Name, created, d.Location)}, nil
		},
	}
}

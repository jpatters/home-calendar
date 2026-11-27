package tools

import (
	"context"

	"github.com/jpatters/home-calendar/internal/agent"
	"github.com/jpatters/home-calendar/internal/gcal"
)

type updateEventArgs struct {
	Calendar      string  `json:"calendar"`
	Title         string  `json:"title"`
	OriginalStart string  `json:"original_start"`
	NewStart      *string `json:"new_start"`
	NewEnd        *string `json:"new_end"`
	NewTitle      *string `json:"new_title"`
	Location      *string `json:"location"`
	AllDay        *bool   `json:"all_day"`
}

func updateEvent(d Deps) agent.Tool {
	return tool[updateEventArgs]{
		name:        "update_event",
		description: "Change an existing event on a Google calendar, found by its title and current start. Null fields are left as they are.",
		schema: object(map[string]any{
			"calendar":       prop("string", "Calendar name."),
			"title":          prop("string", "The event's current title."),
			"original_start": prop("string", "When the event currently starts (the day is enough for an all-day event). "+timeFormatHelp),
			"new_start":      nullable("string", "New start, or null. Moving the start without a new end keeps the duration. "+timeFormatHelp),
			"new_end":        nullable("string", "New end, or null. For all-day events the last day (inclusive). "+timeFormatHelp),
			"new_title":      nullable("string", "New title, or null."),
			"location":       nullable("string", "New location, or null."),
			"all_day":        nullable("boolean", "Make the event all-day (true) or timed (false), or null to keep it. Needs new_start."),
		}),
		fn: func(ctx context.Context, a updateEventArgs) (any, error) {
			cal, calID, err := googleCalendar(d.Config(), a.Calendar)
			if err != nil {
				return nil, err
			}
			around, _, err := parseTime(a.OriginalStart, d.Location)
			if err != nil {
				return nil, err
			}
			patch := gcal.EventPatch{Title: a.NewTitle, Location: a.Location, AllDay: a.AllDay}
			if a.NewStart != nil {
				start, _, err := parseTime(*a.NewStart, d.Location)
				if err != nil {
					return nil, err
				}
				patch.Start = &start
			}
			if a.NewEnd != nil {
				end, _, err := parseTime(*a.NewEnd, d.Location)
				if err != nil {
					return nil, err
				}
				if a.AllDay != nil && *a.AllDay {
					end = end.AddDate(0, 0, 1)
				}
				patch.End = &end
			}
			found, err := d.Calendar.FindEvent(ctx, calID, a.Title, around)
			if err != nil {
				return nil, err
			}
			updated, err := d.Calendar.UpdateEvent(ctx, calID, found.ID, patch)
			if err != nil {
				return nil, err
			}
			return map[string]any{"event": viewGoogleEvent(cal.Name, updated, d.Location)}, nil
		},
	}
}

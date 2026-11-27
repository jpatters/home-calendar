package tools

import (
	"context"

	"github.com/jpatters/home-calendar/internal/agent"
)

type deleteEventArgs struct {
	Calendar string `json:"calendar"`
	Title    string `json:"title"`
	Start    string `json:"start"`
}

func deleteEvent(d Deps) agent.Tool {
	return tool[deleteEventArgs]{
		name:        "delete_event",
		description: "Delete an event from a Google calendar, found by its title and start.",
		schema: object(map[string]any{
			"calendar": prop("string", "Calendar name."),
			"title":    prop("string", "The event's title."),
			"start":    prop("string", "When the event starts: the day alone searches that whole day; a time matches within an hour. "+timeFormatHelp),
		}),
		fn: func(ctx context.Context, a deleteEventArgs) (any, error) {
			cal, calID, err := googleCalendar(d.Config(), a.Calendar)
			if err != nil {
				return nil, err
			}
			from, to, err := lookupWindow(a.Start, d.Location)
			if err != nil {
				return nil, err
			}
			found, err := d.Calendar.FindEvent(ctx, calID, a.Title, from, to)
			if err != nil {
				return nil, err
			}
			if err := d.Calendar.DeleteEvent(ctx, calID, found.ID); err != nil {
				return nil, err
			}
			return map[string]any{"deleted": viewGoogleEvent(cal.Name, found, d.Location)}, nil
		},
	}
}

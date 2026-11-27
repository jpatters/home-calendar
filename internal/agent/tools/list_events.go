package tools

import (
	"context"
	"sort"
	"strings"

	"github.com/jpatters/home-calendar/internal/agent"
	"github.com/jpatters/home-calendar/internal/types"
)

type listEventsArgs struct {
	Start    string  `json:"start"`
	End      string  `json:"end"`
	Calendar *string `json:"calendar"`
}

func listEvents(d Deps) agent.Tool {
	return tool[listEventsArgs]{
		name:        "list_events",
		description: "List calendar events that overlap a time range, optionally on one calendar.",
		schema: object(map[string]any{
			"start":    prop("string", "Start of the range. "+timeFormatHelp),
			"end":      prop("string", "End of the range; a date includes that whole day. "+timeFormatHelp),
			"calendar": nullable("string", "Only list events on this calendar, by name; null for all calendars."),
		}),
		fn: func(ctx context.Context, a listEventsArgs) (any, error) {
			start, _, err := parseTime(a.Start, d.Location)
			if err != nil {
				return nil, err
			}
			end, err := parseEnd(a.End, d.Location)
			if err != nil {
				return nil, err
			}
			calendar := ""
			if a.Calendar != nil {
				cal, err := findCalendar(d.Config(), *a.Calendar)
				if err != nil {
					return nil, err
				}
				calendar = cal.Name
			}
			var source []types.Event
			if d.Events != nil {
				source = append(source, d.Events.Events()...)
			}
			sort.SliceStable(source, func(i, j int) bool { return source[i].Start.Before(source[j].Start) })
			out := []eventView{}
			for _, e := range source {
				if !e.End.After(start) || !e.Start.Before(end) {
					continue
				}
				if calendar != "" && !strings.EqualFold(e.CalendarName, calendar) {
					continue
				}
				out = append(out, viewEvent(e.CalendarName, e.Title, e.Start, e.End, e.AllDay, e.Location, d.Location))
			}
			return map[string]any{"events": out}, nil
		},
	}
}

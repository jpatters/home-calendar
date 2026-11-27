// Package gcal writes to Google Calendar through the Calendar v3 API using a
// service account that the target calendars have been shared with.
package gcal

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

var (
	// ErrNotFound reports that no event matched a lookup.
	ErrNotFound = errors.New("gcal: event not found")
	// ErrAmbiguous reports that more than one event matched a lookup.
	ErrAmbiguous = errors.New("gcal: more than one event matches")
)

const dateLayout = "2006-01-02"

// NewEvent describes an event to create. For all-day events only the dates
// of Start and End are used and End is exclusive, as in the Calendar API.
type NewEvent struct {
	Title       string
	Start       time.Time
	End         time.Time
	AllDay      bool
	Location    string
	Description string
}

// Event is a single event (or a single instance of a recurring event). For
// all-day events End is the exclusive end date.
type Event struct {
	ID          string
	Title       string
	Start       time.Time
	End         time.Time
	AllDay      bool
	Location    string
	Description string
}

// EventPatch lists the fields to change on an existing event; nil fields are
// left as they are. A Start without an End keeps the event's duration. When
// AllDay is nil, Start and End take the event's current all-day-ness (End is
// then the exclusive end date for an all-day event).
type EventPatch struct {
	Title    *string
	Start    *time.Time
	End      *time.Time
	AllDay   *bool
	Location *string
}

// Client creates and edits events on calendars shared with the service
// account.
type Client struct {
	events *calendar.EventsService
}

// CalendarIDFromICalURL extracts the calendar id from a Google Calendar iCal
// feed URL, either the secret form
// (https://calendar.google.com/calendar/ical/<id>/private-<token>/basic.ics)
// or the public one (.../ical/<id>/public/basic.ics).
func CalendarIDFromICalURL(u string) (string, error) {
	parsed, err := url.Parse(u)
	if err != nil {
		return "", fmt.Errorf("gcal: parse iCal URL: %w", err)
	}
	if parsed.Host != "calendar.google.com" {
		return "", fmt.Errorf("gcal: %q is not a Google Calendar iCal URL", u)
	}
	parts := strings.Split(parsed.EscapedPath(), "/")
	if len(parts) != 6 || parts[1] != "calendar" || parts[2] != "ical" || !strings.HasSuffix(parts[5], ".ics") ||
		(parts[4] != "public" && !strings.HasPrefix(parts[4], "private-")) {
		return "", fmt.Errorf("gcal: %q is not a Google Calendar iCal URL", u)
	}
	id, err := url.PathUnescape(parts[3])
	if err != nil || id == "" {
		return "", fmt.Errorf("gcal: %q has no calendar id", u)
	}
	return id, nil
}

// LoadCredentials returns service account JSON from value, which is either
// the JSON itself or a path to a file containing it.
func LoadCredentials(value string) ([]byte, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, errors.New("gcal: no service account credentials configured")
	}
	if strings.HasPrefix(trimmed, "{") {
		return []byte(value), nil
	}
	b, err := os.ReadFile(trimmed)
	if err != nil {
		return nil, fmt.Errorf("gcal: read credentials: %w", err)
	}
	return b, nil
}

// New returns a client authenticated as the service account in
// credentialsJSON. opts are applied after the credentials, so callers can
// override the endpoint or HTTP client.
func New(ctx context.Context, credentialsJSON []byte, opts ...option.ClientOption) (*Client, error) {
	jwt, err := google.JWTConfigFromJSON(credentialsJSON, calendar.CalendarEventsScope)
	if err != nil {
		return nil, fmt.Errorf("gcal: service account credentials: %w", err)
	}
	all := append([]option.ClientOption{option.WithTokenSource(jwt.TokenSource(ctx))}, opts...)
	svc, err := calendar.NewService(ctx, all...)
	if err != nil {
		return nil, fmt.Errorf("gcal: %w", err)
	}
	return &Client{events: svc.Events}, nil
}

func (c *Client) CreateEvent(ctx context.Context, calID string, e NewEvent) (Event, error) {
	created, err := c.events.Insert(calID, &calendar.Event{
		Summary:     e.Title,
		Location:    e.Location,
		Description: e.Description,
		Start:       eventTime(e.Start, e.AllDay, false),
		End:         eventTime(e.End, e.AllDay, false),
	}).Context(ctx).Do()
	if err != nil {
		return Event{}, apiError("create event", err)
	}
	return fromAPI(created)
}

// FindEvent returns the one event titled title (compared case-insensitively,
// ignoring surrounding space) that overlaps [from, to). Recurring events are
// expanded so the result is a single instance.
func (c *Client) FindEvent(ctx context.Context, calID, title string, from, to time.Time) (Event, error) {
	list, err := c.events.List(calID).
		SingleEvents(true).
		Q(title).
		TimeMin(from.Format(time.RFC3339)).
		TimeMax(to.Format(time.RFC3339)).
		Context(ctx).Do()
	if err != nil {
		return Event{}, apiError("list events", err)
	}
	want := strings.TrimSpace(title)
	var matches []Event
	for _, item := range list.Items {
		if !strings.EqualFold(strings.TrimSpace(item.Summary), want) {
			continue
		}
		ev, err := fromAPI(item)
		if err != nil {
			return Event{}, err
		}
		matches = append(matches, ev)
	}
	switch len(matches) {
	case 0:
		return Event{}, fmt.Errorf("%w: %q between %s and %s", ErrNotFound, title, from.Format(time.RFC3339), to.Format(time.RFC3339))
	case 1:
		return matches[0], nil
	}
	candidates := make([]string, len(matches))
	for i, m := range matches {
		when := m.Start.Format("2006-01-02 15:04")
		if m.AllDay {
			when = m.Start.Format(dateLayout) + " (all day)"
		}
		candidates[i] = fmt.Sprintf("%q at %s", m.Title, when)
	}
	return Event{}, fmt.Errorf("%w: %s", ErrAmbiguous, strings.Join(candidates, ", "))
}

func (c *Client) UpdateEvent(ctx context.Context, calID, eventID string, patch EventPatch) (Event, error) {
	body := &calendar.Event{}
	if patch.Title != nil {
		body.Summary = *patch.Title
		body.ForceSendFields = append(body.ForceSendFields, "Summary")
	}
	if patch.Location != nil {
		body.Location = *patch.Location
		body.ForceSendFields = append(body.ForceSendFields, "Location")
	}
	allDay := patch.AllDay != nil && *patch.AllDay
	if patch.AllDay != nil && patch.Start == nil && patch.End == nil {
		return Event{}, errors.New("gcal: changing AllDay requires Start")
	}
	end := patch.End
	keepDuration := patch.Start != nil && end == nil
	if keepDuration || (patch.AllDay == nil && end != nil) {
		existing, err := c.events.Get(calID, eventID).Context(ctx).Do()
		if err != nil {
			return Event{}, apiError("get event", err)
		}
		current, err := fromAPI(existing)
		if err != nil {
			return Event{}, err
		}
		if patch.AllDay == nil {
			allDay = current.AllDay
		}
		if keepDuration {
			e := patch.Start.Add(current.End.Sub(current.Start))
			end = &e
		}
	}
	if patch.Start != nil {
		body.Start = eventTime(*patch.Start, allDay, true)
	}
	if end != nil {
		body.End = eventTime(*end, allDay, true)
	}
	updated, err := c.events.Patch(calID, eventID, body).Context(ctx).Do()
	if err != nil {
		return Event{}, apiError("update event", err)
	}
	return fromAPI(updated)
}

func (c *Client) DeleteEvent(ctx context.Context, calID, eventID string) error {
	if err := c.events.Delete(calID, eventID).Context(ctx).Do(); err != nil {
		return apiError("delete event", err)
	}
	return nil
}

// eventTime converts t to the API's start/end form. Timed values carry the
// UTC offset in dateTime, plus timeZone when t's location has an IANA name.
// For a patch the other form is sent as null, because the API merges a patch
// into the stored start/end rather than replacing it.
func eventTime(t time.Time, allDay, patch bool) *calendar.EventDateTime {
	if allDay {
		dt := &calendar.EventDateTime{Date: t.Format(dateLayout)}
		if patch {
			dt.NullFields = []string{"DateTime", "TimeZone"}
		}
		return dt
	}
	dt := &calendar.EventDateTime{DateTime: t.Format(time.RFC3339)}
	if name := t.Location().String(); name != "Local" && name != "" {
		dt.TimeZone = name
	}
	if patch {
		dt.NullFields = []string{"Date"}
	}
	return dt
}

func fromAPI(e *calendar.Event) (Event, error) {
	start, allDay, err := parseEventTime(e.Start)
	if err != nil {
		return Event{}, fmt.Errorf("gcal: event %s start: %w", e.Id, err)
	}
	end, _, err := parseEventTime(e.End)
	if err != nil {
		return Event{}, fmt.Errorf("gcal: event %s end: %w", e.Id, err)
	}
	return Event{
		ID:          e.Id,
		Title:       e.Summary,
		Start:       start,
		End:         end,
		AllDay:      allDay,
		Location:    e.Location,
		Description: e.Description,
	}, nil
}

func parseEventTime(dt *calendar.EventDateTime) (time.Time, bool, error) {
	if dt == nil {
		return time.Time{}, false, errors.New("missing")
	}
	if dt.DateTime != "" {
		t, err := time.Parse(time.RFC3339, dt.DateTime)
		return t, false, err
	}
	t, err := time.ParseInLocation(dateLayout, dt.Date, time.Local)
	return t, true, err
}

func apiError(op string, err error) error {
	var gerr *googleapi.Error
	if errors.As(err, &gerr) && (gerr.Code == http.StatusNotFound || gerr.Code == http.StatusGone) {
		return fmt.Errorf("%w: %s: %v", ErrNotFound, op, err)
	}
	return fmt.Errorf("gcal: %s: %w", op, err)
}

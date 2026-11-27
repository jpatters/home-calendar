# Noridoc: gcal

Path: @/internal/gcal

### Overview

- A thin Google Calendar v3 client (`google.golang.org/api/calendar/v3`) used only for *writing* events: create, find, update and delete. Reading the calendar for the display still goes through the iCal feeds in @/internal/ical.
- Authenticates as a Google service account (`golang.org/x/oauth2/google` JWT, `CalendarEventsScope`). It can only touch calendars that have been shared with the service account's email with "Make changes to events".
- Bridges the two worlds with `CalendarIDFromICalURL`, which pulls the Google calendar id out of the iCal URL already stored in config, so no extra per-calendar setting is needed.

### How it fits into the larger codebase

- Constructed in @/internal/server/agent.go (`googleCalendarClient`): `GOOGLE_SERVICE_ACCOUNT_JSON` is passed to `LoadCredentials`, then `New`. If the variable is empty or either step fails, the server logs it and runs the assistant without calendar writes (`CalendarWritable=false`), rather than failing startup.
- The only consumer is the assistant's calendar tools in @/internal/agent/tools, which depend on the `CalendarWriter` interface (satisfied by `*Client`). Those tools resolve a calendar *name* from config, call `CalendarIDFromICalURL` on its URL, and then call this client.
- Writes do not update the display directly. The change appears on the wall once the iCal fetcher (@/internal/ical) next polls the feed.

### Core Implementation

- `CalendarIDFromICalURL` accepts only `calendar.google.com` URLs of the shape `/calendar/ical/<id>/private-<token>/basic.ics` or `/calendar/ical/<id>/public/basic.ics`, and returns the path-unescaped `<id>` (e.g. `family@group.calendar.google.com`). Anything else is an error, which the tools surface as "this calendar can't be changed from here".
- `LoadCredentials(value)`: a value whose trimmed form starts with `{` is the JSON itself; otherwise it is treated as a file path and read. This lets the same env var hold inline JSON or point at a mounted secret file.
- `New(ctx, json, opts...)` applies caller options after the token source, which is how tests redirect the endpoint and HTTP client to an `httptest` server.
- `FindEvent(calID, title, from, to)` lists events with `SingleEvents(true)` (recurring events expanded to instances) and full-text `Q(title)` overlapping the caller's `[from, to)` window, then keeps only those whose summary equals the title case-insensitively (trimmed). Zero matches is `ErrNotFound`; more than one is `ErrAmbiguous` with the candidates' titles and start times in the message, so the model can ask which one was meant.
- `UpdateEvent` has patch semantics (`Events.Patch`): only non-nil `EventPatch` fields are sent, with `ForceSendFields` so an empty title/location is still sent. If `Start` moves without an `End`, the current event is fetched and its duration is preserved. Whenever `Start` or `End` is given without `AllDay`, the current event is fetched so its timed/all-day kind is kept (`End` is then exclusive for an all-day event). Changing `AllDay` without a new `Start` or `End` is rejected.
- `eventTime` builds the API's start/end: all-day uses `Date`, timed uses RFC3339 `DateTime` plus `TimeZone` when the time's location has an IANA name. For patches, the other form is sent as an explicit null (`NullFields`), because the API merges patch start/end into the stored value; without it, switching timed <-> all-day would leave both `date` and `dateTime` set.
- `apiError` maps Google 404/410 to a wrapped `ErrNotFound`; other failures are wrapped as `gcal: <op>: ...`.

### Things to Know

- All-day `End` is exclusive everywhere in this package, as in the Calendar API. The inclusive "last day" convention the model sees is applied in @/internal/agent/tools, not here.
- All-day dates returned by Google are parsed in `time.Local`; timed values keep the offset from the API response.
- Update and delete always look the event up by title + start first, so the assistant never needs (or exposes) Google event ids.

Created and maintained by Nori.

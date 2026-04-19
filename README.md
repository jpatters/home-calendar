# home-calendar

A touchscreen calendar dashboard for a homelab wall/desk display. Aggregates one
or more Google Calendars (via their public iCal share URLs) and shows them in
day / week / month views alongside a clock and weather widget. Everything is
configured from a bundled admin page. No user accounts — intended for a trusted
private network.

![layout](docs/layout.txt) <!-- optional -->

## Stack

- **Backend**: Go, standard library `net/http`, `github.com/arran4/golang-ical`
  for iCal parsing, `github.com/coder/websocket` for live updates. Single
  static binary, frontend bundled via `//go:embed`.
- **Frontend**: React 18 + Vite + TypeScript, FullCalendar (day/week/month +
  touch), React Router.
- **Calendars**: Google Calendar "Secret address in iCal format" URLs.
- **Weather**: [Open-Meteo](https://open-meteo.com/) — no API key.
- **Snow day predictor**: optional widget backed by
  [snowdaypredictor.com](https://www.snowdaypredictor.com/) — paste a location
  page URL.
- **Config**: single JSON file on a bind-mounted volume.
- **Live updates**: one WebSocket (`/api/ws`), no polling.

## Quick start (Docker)

```bash
docker compose up --build -d
```

Open:

- `http://<host>:8080/` — touchscreen display
- `http://<host>:8080/admin` — configuration page

On first run the app seeds an empty config at `./data/config.json`. Go to
`/admin`, add your Google Calendar iCal URLs, set your weather latitude/longitude,
and save. Changes are broadcast to the display instantly over the WebSocket.

### Getting a Google Calendar iCal URL

In Google Calendar, open the calendar's settings → "Integrate calendar" → copy
the **Secret address in iCal format**. Paste that into the admin page. This
works for personal accounts without any OAuth setup.

## Local development

Backend:

```bash
go run ./cmd/server
# CONFIG_PATH=./data/config.json LISTEN_ADDR=:8080 go run ./cmd/server
```

Frontend (proxies `/api` and `/api/ws` to `:8080`):

```bash
cd web
npm ci
npm run dev           # http://localhost:5173
```

A fresh checkout ships with a placeholder `index.html` under
`internal/server/dist/` so `go build` always works. The Docker build replaces it
with the real Vite output. To embed a freshly built frontend in a local `go
build`, run:

```bash
npm --prefix web ci
npm --prefix web run build
rm -rf internal/server/dist && cp -r web/dist internal/server/dist
go build ./cmd/server
```

## Configuration

Stored in `CONFIG_PATH` (default `/data/config.json` in Docker). Shape:

```json
{
  "calendars": [
    {
      "id": "uuid-generated-automatically",
      "name": "Family",
      "color": "#4285f4",
      "url": "https://calendar.google.com/calendar/ical/.../basic.ics"
    }
  ],
  "weather": {
    "latitude": 43.65,
    "longitude": -79.38,
    "units": "metric",
    "timezone": "auto"
  },
  "snowDay": {
    "url": "https://www.snowdaypredictor.com/prediction/canoe-cove-pe"
  },
  "display": {
    "defaultView": "week",
    "calendarRefreshSeconds": 300,
    "weatherRefreshSeconds": 900,
    "theme": "light"
  }
}
```

Edit via `/admin` rather than by hand — the admin UI performs validation,
assigns IDs, and triggers an immediate refresh.

## API

| Method | Path                          | Purpose                                   |
|--------|-------------------------------|-------------------------------------------|
| GET    | `/api/config`                 | Return current config                     |
| PUT    | `/api/config`                 | Replace config; broadcasts `config` frame |
| GET    | `/api/calendar/events`        | Current merged events from cache          |
| POST   | `/api/calendar/refresh`       | Force an iCal refresh                     |
| GET    | `/api/weather`                | Current weather snapshot                  |
| POST   | `/api/weather/refresh`        | Force a weather refresh                   |
| GET    | `/api/snowday`                | Current snow day prediction snapshot      |
| POST   | `/api/snowday/refresh`        | Force a snow day refresh                  |
| GET    | `/api/ws`                     | WebSocket: snapshot + live updates        |

### WebSocket frames

```json
{ "type": "snapshot", "config": {...}, "events": [...], "weather": {...}, "snowday": {...} }
{ "type": "calendar", "events": [...] }
{ "type": "weather",  "weather": {...} }
{ "type": "snowday",  "snowday": {...} }
{ "type": "config",   "config": {...} }
```

On connect the client receives a `snapshot` with everything it needs to render.
Subsequent frames arrive whenever a background fetcher updates the cache or the
admin saves config. The client reconnects with exponential backoff.

## Environment variables

| Var           | Default               | Description                 |
|---------------|-----------------------|-----------------------------|
| `CONFIG_PATH` | `/data/config.json`   | Config JSON path            |
| `LISTEN_ADDR` | `:8080`               | Listen address              |

## Notes

- No authentication. Run behind your homelab's trust boundary.
- RRULE handling supports the common `FREQ=DAILY/WEEKLY/MONTHLY/YEARLY` +
  `INTERVAL` / `COUNT` / `UNTIL`. Uncommon RRULE forms fall back to the single
  occurrence.
- Events are expanded over a window from 3 months back to 12 months ahead.

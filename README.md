# home-calendar

A touchscreen calendar dashboard for a homelab wall/desk display. Aggregates one
or more Google Calendars (via their public iCal share URLs) and shows them in
day / week / month views alongside clock, weather, tide, sports, pool and hot
tub widgets, plus an optional voice assistant. Everything is
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
- **Tides**: [CHS IWLS](https://api-iwls.dfo-mpo.gc.ca/swagger-ui/index.html),
  the Canadian Hydrographic Service tide-station service — no API key. Reports
  the harmonic high/low predictions published for a chosen station, so heights
  and times match Canadian tide tables. Heights are metres above chart datum.
  **Canadian stations only.**
- **Snow day predictor**: optional widget backed by
  [snowdaypredictor.com](https://www.snowdaypredictor.com/) — paste a location
  page URL.
- **Baseball**: optional widget showing the most recent completed MLB game's
  score and the date/time/opponent of the next upcoming game. When the team is
  currently playing, the widget shows a "LIVE" pill with the running score,
  inning, and outs in place of the latest-final section, and the server polls
  the schedule API every 30 seconds while the game is live (10-minute cadence
  otherwise). Powered by the free, no-auth MLB Stats API
  (`statsapi.mlb.com`). Pick a team via type-ahead search in admin. Regular
  season + playoffs only; spring training and exhibition games are filtered
  out.
- **Hockey**: optional widget showing the most recent completed NHL game's
  score (marked `Final/OT` or `Final/SO` when decided past regulation) and the
  date/time/opponent of the next game. While the team is playing, the widget
  shows a "LIVE" pill with the running score, period, and clock (or
  "Intermission"), and the server polls every 30 seconds instead of the usual
  10 minutes. Powered by the free, no-auth NHL web API (`api-web.nhle.com`):
  the team scoreboard for recent and upcoming games, plus the gamecenter
  landing for the clock during a live game. Pick a team via type-ahead search
  in admin; teams are keyed by their three-letter abbreviation. Regular season
  + playoffs only; preseason games and anything not on a confirmed schedule
  (postponed, cancelled, time TBD) are filtered out.
- **Pool**: optional widget showing the pool's current water temperature and
  whether the heat pump is actively heating or idle. Reads the `climate` entity
  from an [ESPHome](https://esphome.io/) device's built-in web server over the
  LAN — no cloud, no API key. Set the device URL in admin (a fixed IP is more
  reliable than an mDNS `.local` name from a container). Tap the widget to
  open a panel with up/down arrows that change the target half a degree
  Celsius per tap; a burst of taps is sent as a single write, and the arrows
  stop at the setpoint limits the thermostat reports (10–40 °C if it reports
  none).
- **Hot tub**: optional widget showing the hot tub's water temperature,
  setpoint, and whether the heater is on. Talks directly to a Gecko in.touch2
  WiFi module over the LAN using its UDP protocol on port 10022 (the same one
  the in.touch2 phone app and the geckolib Home Assistant integration use) — no
  cloud, no API key. Only the inYT spa pack with log structure v66 is
  supported; other packs are rejected with a clear error rather than decoded
  with the wrong byte offsets. Temperatures are shown in whole degrees
  Fahrenheit to match the tub's own panel. Tap the widget to open a panel with
  up/down arrows that change the target one degree per tap; a burst of taps is
  sent to the tub as a single write, and the arrows stop at the limits the spa
  pack reports (typically 59–106 °F). Set the module's IP in admin. This
  is unicast UDP to a fixed address, so it works from the default Docker bridge
  network without host networking.
- **Assistant (Ziggy)**: optional voice assistant that can answer questions
  about the calendar, live game scores and the weather, add/change/delete
  calendar events, and read or change the hot tub and pool temperatures. It
  runs on the OpenAI Responses API, using your ChatGPT subscription via
  **Sign in with ChatGPT** or, as a fallback, an OpenAI API key — both set up
  in the admin **Assistant** tab, along with the model and reasoning effort
  (default `gpt-6.1-terra`, effort `low`). Voice in and
  out goes through Home Assistant (see
  [Assistant setup](#assistant-setup-ziggy)). On the display Ziggy is an 8-bit
  character living in a pixel-art room panel: it thinks, talks (with a speech
  bubble), sleeps during quiet hours, dresses for the season and for Canadian
  holidays (from two days before each holiday), and its window shows the time
  of day and the current weather. Optionally, people can tap Ziggy to type a
  message instead of speaking.
- **Config**: single JSON file on a bind-mounted volume. The assistant's
  credentials live in a separate, private `secrets.json` next to it.
- **Live updates**: one WebSocket (`/api/ws`), no polling.
- **Layout**: the display is a [dockview](https://dockview.dev/) layout —
  calendar on the left and widgets stacked in a column on the right by default.
  It is locked during normal use. Tap **Edit layout** in the footer bar to show
  tabs, then drag tabs to move, stack, or split widgets (on touch, long-press
  to start a drag) and drag the dividers to resize; **Done** saves the layout
  to the server config and every connected display picks it up. **Reset
  layout** returns to the default arrangement.
- **Refresh**: a small button in the footer bar reloads the whole page, for
  when a wall display needs a kick.

## Quick start (Docker)

```bash
docker compose up --build -d
```

Open:

- `http://<host>:8080/` — touchscreen display
- `http://<host>:8080/admin` — configuration page

The bundled `docker-compose.yml` publishes the container's port `8080` on host
port `9090`, so with it use `http://<host>:9090/` instead (and in the Home
Assistant URL below).

### Pulling the prebuilt image from GHCR

Every push to `main` and every `v*.*.*` tag publishes a container image to the
GitHub Container Registry via `.github/workflows/docker-publish.yml`. After the
first successful run you'll need to flip the package from private to public
**once** (this can't be automated with `GITHUB_TOKEN`):

1. Open `https://github.com/users/jpatters/packages/container/home-calendar/settings`
2. Scroll to **Danger Zone** → **Change visibility** → **Public**

From then on you can pull without auth:

```bash
docker pull ghcr.io/jpatters/home-calendar:latest  # tracks main
docker pull ghcr.io/jpatters/home-calendar:v1.2.3  # pinned release
```

Images are `linux/amd64` only.

On first run the app seeds an empty config at `./data/config.json`. Go to
`/admin`, add your Google Calendar iCal URLs, search for your city in the
**Weather** section (pick sections from the sidebar, or the ☰ menu on a
phone), and save. Changes are broadcast to the display instantly over
the WebSocket.

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
    "enabled": true,
    "latitude": 43.65,
    "longitude": -79.38,
    "units": "metric",
    "timezone": "auto",
    "location": "Toronto, Ontario, Canada"
  },
  "tide": {
    "enabled": true,
    "stationCode": "07120",
    "units": "metric",
    "location": "Victoria"
  },
  "snowDay": {
    "enabled": true,
    "url": "https://www.snowdaypredictor.com/prediction/canoe-cove-pe"
  },
  "baseball": {
    "enabled": true,
    "teamId": 147,
    "teamName": "New York Yankees",
    "teamAbbr": "NYY"
  },
  "hockey": {
    "enabled": true,
    "teamAbbr": "TOR",
    "teamName": "Toronto Maple Leafs"
  },
  "pool": {
    "enabled": true,
    "deviceUrl": "http://pool-thermostat.local"
  },
  "hotTub": {
    "enabled": false,
    "host": "192.168.1.50"
  },
  "agent": {
    "enabled": false,
    "model": "gpt-6.1-terra",
    "effort": "low",
    "textInputEnabled": false,
    "quietStart": "22:00",
    "quietEnd": "07:00"
  },
  "display": {
    "defaultView": "week",
    "calendarRefreshSeconds": 300,
    "weatherRefreshSeconds": 900,
    "tideRefreshSeconds": 600,
    "baseballRefreshSeconds": 600,
    "hockeyRefreshSeconds": 600,
    "poolRefreshSeconds": 30,
    "hotTubRefreshSeconds": 30,
    "theme": "default",
    "mode": "light",
    "calendarEnabled": true,
    "clockEnabled": true
  },
  "layout": { "...": "dockview layout, managed by the display" }
}
```

`layout` is optional and written only by the display's **Edit layout** /
**Done** buttons (via `PUT /api/layout`). The server treats it as opaque, and
saving the config from admin never changes it. Omit it, or press **Reset
layout**, to get the default arrangement.

Each widget has an `enabled` flag. Weather, Tide, Snow Day, Baseball, Hockey,
Pool, Hot Tub, and the assistant each carry their own `enabled` field; the calendar and clock live under
`display.calendarEnabled` and `display.clockEnabled`. When a widget is disabled
the server stops its background fetcher (so no API calls are made) and the
display hides it. You can toggle widgets from the admin page — open the widget's
section from the sidebar and use the "Show …" checkbox at the top (the calendar
and clock toggles are in the **Display** section). Configs written before these flags
existed are treated as all-enabled when loaded, with two exceptions: the hot
tub widget and the assistant default to disabled, and configs from before they
existed keep them off until you turn them on in admin.

`agent` configures Ziggy. `model` and `effort` are passed to OpenAI (`effort`
is one of `none`, `minimal`, `low`, `medium`, `high`, `xhigh`, `max`); a blank
model or unknown effort falls back to the defaults. `quietStart`/`quietEnd`
(24-hour `HH:MM`, may wrap midnight) are when the character sleeps on screen;
it still answers if spoken to. `textInputEnabled` adds the tap-to-type box on
the display. Credentials are **never** stored in the config — it is served to
every browser without authentication. They live in `secrets.json` next to
`config.json` — see [Assistant setup](#assistant-setup-ziggy).

The hot tub `host` is the in.touch2 module's LAN address as `ip` or `ip:port`;
the port defaults to `10022`.

The baseball widget additionally stays dormant while no team is selected
(`teamId: 0`); no polling happens until you pick a team from the admin
type-ahead search.

The tide widget behaves the same way while `stationCode` is empty. Pick a
station from the admin type-ahead, which searches CHS stations by name or code
and offers only those publishing both the high/low and continuous prediction
series. Coordinates are not used: the nearest station by distance is often the
wrong one across a headland or on the far side of a strait, so the station is
always chosen explicitly.

`theme` is the colour palette — one of `default`, `ocean`, `sunset`, `forest`.
`mode` is one of `light`, `dark`, or `auto`. `auto` flips between light and dark
based on the configured weather location's sunrise/sunset (so the display
follows your local sun). It requires weather to be enabled with a valid
location. Configs from earlier versions that used `"theme": "light"` or
`"theme": "dark"` are migrated automatically the first time the server reads
them — they become `"theme": "default"` with the corresponding `mode`.

Edit via `/admin` rather than by hand — the admin UI performs validation,
assigns IDs, and triggers an immediate refresh.

## API

| Method | Path                          | Purpose                                   |
|--------|-------------------------------|-------------------------------------------|
| GET    | `/api/config`                 | Return current config                     |
| PUT    | `/api/config`                 | Replace config; broadcasts `config` frame |
| PUT    | `/api/layout`                 | Save display layout (object, or `null` to reset); broadcasts `config` frame |
| GET    | `/api/calendar/events`        | Current merged events from cache          |
| POST   | `/api/calendar/refresh`       | Force an iCal refresh                     |
| GET    | `/api/weather`                | Current weather snapshot                  |
| POST   | `/api/weather/refresh`        | Force a weather refresh                   |
| GET    | `/api/weather/geocode?q=`     | Search Open-Meteo geocoding for a place   |
| GET    | `/api/snowday`                | Current snow day prediction snapshot      |
| POST   | `/api/snowday/refresh`        | Force a snow day refresh                  |
| GET    | `/api/tide`                   | Current tide snapshot                     |
| POST   | `/api/tide/refresh`           | Force a tide refresh                      |
| GET    | `/api/tide/stations?q=`       | Tide station type-ahead search (CHS)      |
| GET    | `/api/baseball`               | Current baseball snapshot                 |
| POST   | `/api/baseball/refresh`       | Force a baseball refresh                  |
| GET    | `/api/baseball/teams?q=`      | Team type-ahead search (MLB)              |
| GET    | `/api/hockey`                 | Current hockey snapshot                   |
| POST   | `/api/hockey/refresh`         | Force a hockey refresh                    |
| GET    | `/api/hockey/teams?q=`        | Team type-ahead search (NHL)              |
| GET    | `/api/pool`                   | Current pool temperature snapshot         |
| POST   | `/api/pool/refresh`           | Force a pool refresh                      |
| POST   | `/api/pool/target`            | Set the pool target (`{"targetC": 28.5}`) |
| GET    | `/api/hottub`                 | Current hot tub temperature snapshot      |
| POST   | `/api/hottub/refresh`         | Force a hot tub refresh                   |
| POST   | `/api/hottub/target`          | Set the hot tub target (`{"targetF": 103}`) |
| GET    | `/api/agent`                  | Assistant state, plus whether it has a credential and calendar write access |
| POST   | `/api/agent/converse`         | Talk to Ziggy (Home Assistant Webhook Conversation contract) |
| GET    | `/api/agent/auth`             | Which credential Ziggy uses and what is configured (never secret values) |
| POST   | `/api/agent/auth/chatgpt/start`    | Start Sign in with ChatGPT; returns `{"authorizeUrl"}` |
| POST   | `/api/agent/auth/chatgpt/complete` | Finish sign-in (`{"redirectUrl": "<pasted address>"}`) |
| DELETE | `/api/agent/auth/chatgpt`     | Sign out of ChatGPT (revokes the sign-in) |
| PUT/DELETE | `/api/agent/auth/api-key` | Save (`{"key": "..."}`) or clear the OpenAI API key |
| PUT/DELETE | `/api/agent/auth/google`  | Save (`{"json": "..."}`) or clear the Google service account key |
| GET    | `/api/ws`                     | WebSocket: snapshot + live updates        |

The `POST …/refresh` endpoints return `409 Conflict` when the corresponding
widget is disabled.

`POST /api/hottub/target` takes a whole number of degrees Fahrenheit within the
`minTargetF`–`maxTargetF` range the snapshot reports. It returns the fresh
snapshot once the module acknowledges the write and broadcasts it as a `hottub`
frame; `400` for a fractional or out-of-range target, `409` when the widget is
disabled, and `502` when the module does not answer.

`POST /api/pool/target` works the same way in degrees Celsius: the target must
lie within the `minTargetC`–`maxTargetC` range the snapshot reports (`400`
otherwise), and the fresh reading is broadcast as a `pool` frame.

`POST /api/agent/converse` takes `{"conversation_id": "...", "query": "..."}`
and answers `200 {"output": "..."}` with the text to speak. It answers `200`
even when the assistant is disabled, isn't connected, or fails — the `output`
then explains the problem — because Home Assistant speaks nothing on an error
status. Only a missing or blank `query` gets `400`. Follow-ups within five
minutes on the same `conversation_id` continue the conversation.

The `/api/agent/auth` endpoints answer with the same status object on success
and `{"error": "..."}` otherwise; changing a credential that an environment
variable sets returns `409`.

### WebSocket frames

```json
{ "type": "snapshot", "config": {...}, "events": [...], "weather": {...}, "snowday": {...}, "tide": {...}, "baseball": {...}, "hockey": {...}, "pool": {...}, "hottub": {...}, "agent": {...} }
{ "type": "calendar", "events": [...] }
{ "type": "weather",  "weather": {...} }
{ "type": "snowday",  "snowday": {...} }
{ "type": "tide",     "tide": {...} }
{ "type": "baseball", "baseball": {...} }
{ "type": "hockey",   "hockey": {...} }
{ "type": "pool",     "pool": {...} }
{ "type": "hottub",   "hottub": {...} }
{ "type": "agent",    "agent": {...} }
{ "type": "config",   "config": {...} }
```

When a widget is disabled via config save, the server broadcasts its frame type
with a `null` payload so clients can drop any stale snapshot immediately.

On connect the client receives a `snapshot` with everything it needs to render.
Subsequent frames arrive whenever a background fetcher updates the cache or the
admin saves config. The client reconnects with exponential backoff.

## Environment variables

| Var                           | Default             | Description |
|-------------------------------|---------------------|-------------|
| `CONFIG_PATH`                 | `/data/config.json` | Config JSON path |
| `LISTEN_ADDR`                 | `:8080`             | Listen address |
| `OPENAI_API_KEY`              | (none)              | Optional. Overrides the API key saved in admin (it then can't be changed there). |
| `GOOGLE_SERVICE_ACCOUNT_JSON` | (none)              | Optional. Overrides the Google service account saved in admin, as the JSON itself or a path to the file. |
| `TZ`                          | UTC                 | IANA time zone (e.g. `America/Halifax`). Set it so Ziggy reads and books times in local time. |

## Assistant setup (Ziggy)

Everything is configured in the admin **Assistant** tab. The **Connection**
area shows what Ziggy is using ("Using: ChatGPT subscription", "Using: API
key" or "Not connected").

1. **Time zone.** Set `TZ` (e.g. `TZ=America/Halifax`) in the server's
   environment so Ziggy uses local time; without it the container runs in UTC
   and events are booked in UTC.
2. **Connect to OpenAI** — either one; a ChatGPT sign-in is used when both are
   set.
   - **Sign in with ChatGPT (uses your ChatGPT plan).** Click **Sign in with
     ChatGPT**; a sign-in tab opens at OpenAI. After you sign in and allow
     access, that tab ends on a page that can't load (an address starting
     `http://127.0.0.1:1455/auth/callback`). That's expected: copy the tab's
     whole address, paste it into the admin field and press **Finish
     sign-in**. This works from any device, including the wall tablet or a
     phone. **Sign out** revokes the sign-in; if OpenAI doesn't confirm, admin
     says so and you can disconnect Home Calendar in ChatGPT's settings.
   - **OpenAI API key (fallback).** Paste a key and press **Save**. It is
     write-only: admin only shows whether one is saved.

   Ziggy uses the signed-in ChatGPT plan for **everyone** who talks to it.
   OpenAI's Sign in with ChatGPT terms expect a plan to be used only for the
   signed-in person's own activity, so if others in the house will talk to
   Ziggy, use an API key instead.
3. **Calendar changes (optional).** Without this Ziggy can read the calendars
   but not change them.
   - In Google Cloud, enable the Google Calendar API, create a service account,
     and download a JSON key for it.
   - Paste the whole JSON key into **Google Calendar** in admin and press
     **Save**. Admin then shows the service account's email address.
   - In Google Calendar, share **each** calendar Ziggy should be able to edit
     with that email address, with the permission **Make changes to events**.
   - Calendars are matched by the names you gave them in admin, and the Google
     calendar is identified from its iCal URL, so only calendars added with a
     Google Calendar iCal address can be changed.
4. **Turn Ziggy on** in the admin **Assistant** tab, pick the model,
   reasoning effort and quiet hours, and press **Save**. (Connection changes
   apply immediately and don't need Save.)
5. **Home Assistant (voice).**
   - Install the **Webhook Conversation** integration via HACS (needs Home
     Assistant 2026.4 or newer).
   - Add a conversation agent with URL
     `http://<host>:8080/api/agent/converse`, output field `output`,
     streaming **off**, and a 30-second timeout. (Use the host port you mapped
     in `docker-compose.yml`.)
   - Under **Settings → Voice assistants**, choose that agent as the
     conversation agent of an Assist pipeline.
   - The "Hey Ziggy" wake word is configured on the voice device / Home
     Assistant side; this app never hears audio.

Credentials are stored in `secrets.json` next to `config.json` (with Docker,
`/data/secrets.json`), readable only by the server's user. Back it up along
with `config.json` and keep it private. The environment variables
`OPENAI_API_KEY` and `GOOGLE_SERVICE_ACCOUNT_JSON` still work as optional
overrides; when one is set, admin shows that field as set by the environment
and it can't be edited there.

Each request has 25 seconds to answer, so Home Assistant's 30-second timeout
is never hit; a slow or failed answer is spoken as a short apology, and Ziggy
says so if the ChatGPT plan's usage limit is reached or it needs signing in
again. Conversation history (for follow-up questions) is kept only in the
server's memory for five minutes after the last reply; requests ask OpenAI not
to store anything.

## Notes

- No authentication. Run behind your homelab's trust boundary.
- RRULE handling supports the common `FREQ=DAILY/WEEKLY/MONTHLY/YEARLY` +
  `INTERVAL` / `COUNT` / `UNTIL`. Uncommon RRULE forms fall back to the single
  occurrence.
- Events are expanded over a window from 3 months back to 12 months ahead.

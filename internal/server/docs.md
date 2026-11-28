# Noridoc: server

Path: @/internal/server

### Overview

- The HTTP and WebSocket layer of home-calendar. It owns one `Fetcher` per widget (calendar, weather, snow day, tide, baseball, hockey, pool, hot tub), the config store, the secrets store (@/internal/secrets), the Ziggy assistant (@/internal/agent) with its ChatGPT sign-in client (@/internal/chatgpt), and a `Hub` that fans out live-data frames to every connected display.
- Serves a JSON API under `/api/*` for the admin UI and the display, a single `/api/ws` WebSocket for push updates, and the embedded Vite build for everything else.
- Started once from @/cmd/server/main.go via `New(ctx, Options{Config, Secrets, ChatGPTIssuer})`; `openStores` there opens `config.json` and the `secrets.json` beside it. `Shutdown()` stops all fetchers.

### How it fits into the larger codebase

- Every widget package under @/internal (e.g. @/internal/hottub, @/internal/tide, @/internal/baseball, @/internal/hockey) exposes the same fetcher contract: `New(onUpdate)`, `Start`, `Stop`, `RefreshNow`, `Snapshot`. `router.go` wires each one's `onUpdate` callback to `hub.Broadcast` with a typed `Frame`, so widget packages never know about WebSockets and the server never knows how data is fetched.
- The two sports widgets are structurally twins. @/internal/baseball polls the MLB Stats API keyed by an integer `TeamID`; @/internal/hockey polls `api-web.nhle.com` keyed by the three-letter `TeamAbbr` (every NHL endpoint is addressed by abbreviation, so there is no numeric id in `types.Hockey`). Both take a normal interval from `Display` config and a fixed 30 s live interval that the fetcher switches to on its own whenever its snapshot holds a live game.
- Configuration lives in @/internal/config (`Store`), typed by @/internal/types. `PUT /api/config` replaces the config, calls `restartFetchers`, and broadcasts a `config` frame; `applyFetcherConfig` is the single place that decides which fetchers run based on each widget's `Enabled` flag.
- The frontend in @/web/src consumes this layer through @/web/src/api.ts (REST) and @/web/src/useLiveData.ts (WebSocket). The `Frame` JSON shape in `hub.go` is mirrored by `WSFrame` in @/web/src/types.ts.
- The Go binary embeds `dist/` (the Vite output) through `static.go`; the SPA handler falls back to `index.html` for unknown paths so React Router routes survive a hard refresh.

```
 admin / display (browser)
    |  REST /api/*            |  WS /api/ws
    v                         v
 handlers.go            ws.go (handleWS)
    |                         |  initial "snapshot" frame, then hub.send
    v                         v
 Server{cfg, fetchers, hub} <--- Hub.Broadcast <--- fetcher.onUpdate callbacks
    |
    v
 internal/{ical,weather,tide,baseball,hockey,pool,hottub,snowday}.Fetcher
    ^
    |  tools read snapshots / call SetTarget
 agent.Agent  <--- POST /api/agent/converse (Home Assistant, display text box)
    |  OnState --> Hub.Broadcast("agent" frame)
    `--> OpenAI Responses API, internal/gcal (Google Calendar writes)

 admin Assistant section --REST /api/agent/auth/*--> agent_auth.go
    |  secrets.Store Set*/Clear*, chatgpt.Client Start/Complete/SignOut
    v
 refreshAgent --> agent.SetCapabilities --> Hub.Broadcast("agent" frame)
```

### Core Implementation

- `Server` struct (`router.go`) holds the config store, one fetcher per widget, the `Hub`, the root context used to restart fetchers, and injectable search functions (`geocode`, `teamSearch` for MLB, `hockeyTeamSearch` for NHL, `stationSearch` for tides) so tests can stub upstream HTTP. In production each search func is bound to the matching fetcher's `HTTPClient()` and default base URL.
- Fetcher lifecycle: `applyFetcherConfig(ctx, cfg, broadcastClears)` starts enabled fetchers with their per-widget refresh interval from `Display` config and stops disabled ones. When called from a config save (`broadcastClears=true`) it also broadcasts a nil snapshot for disabled widgets, and for tide/baseball/hockey it clears *before* starting so a stale snapshot is not relabelled with the new station/team. The sports widgets count as "disabled" when no team is chosen (`TeamID == 0` / `TeamAbbr == ""`), not just when `Enabled` is false.
- Handler conventions (`handlers.go`): `GET /api/<widget>` returns the cached snapshot or JSON `null`; `POST /api/<widget>/refresh` returns 409 when the widget is disabled (or, for baseball/hockey, when no team is selected), otherwise calls `RefreshNow` under a 20 s (45 s for calendars) timeout and returns `{"ok":true}`; search endpoints (`/api/weather/geocode`, `/api/baseball/teams`, `/api/hockey/teams`, `/api/tide/stations`) validate `q`, cap its length, and map upstream errors to 502 without echoing the upstream message.
- Layout write path: `PUT /api/layout` (`handlePutLayout`) takes the display's serialized dockview layout. The body must decode as JSON and be either an object or `null` (anything else is 400); `null` clears the stored layout so displays fall back to their default arrangement. It saves via `Store.SetLayout`, broadcasts a `config` frame with the updated config, and returns that config. Unlike `PUT /api/config` it does not call `restartFetchers`, since only the arrangement changed.
- Hot tub write path: `POST /api/hottub/target` takes `{"targetF": <number>}`. It returns 409 if the widget is disabled, 400 when the body is not JSON or `targetF` is missing, 400 when the driver reports `hottub.ErrInvalidTarget` (fractional or outside the tub's min/max), 502 when the module fails to acknowledge, and 200 with the post-write `HotTubSnapshot` on success. The driver's `Fetcher.SetTarget` also fires `onUpdate`, so the hub broadcasts the same snapshot to all displays independently of the HTTP response.
- WebSocket (`ws.go`): on connect the client is registered with the hub and immediately sent a `snapshot` frame containing the current config plus every fetcher's cached snapshot; after that it only receives what the hub broadcasts. A reader goroutine detects disconnects, and a 30 s ping keeps the connection alive.
- Pool write path: `POST /api/pool/target` takes `{"targetC": <number>}` and mirrors the hot tub endpoint: 409 when disabled, 400 for a bad body or `pool.ErrInvalidTarget` (outside the min/max the ESPHome thermostat reports, default 10-40 °C), 502 when the device fails, 200 with the post-write `PoolSnapshot`. @/internal/pool `SetTarget` reads the entity (`?detail=all`, the only form that includes `min_temp`/`max_temp`), validates, `POST`s `/climate/Pool Heater/set?target_temperature=`, re-reads, and publishes through `onUpdate` so all displays get a `pool` frame.
- Assistant (`agent.go`): `newAgent` builds the @/internal/chatgpt `Client` (with `OnChange: refreshAgent`) and the @/internal/agent `Agent` once in `New`, then calls `refreshAgent`. Its `OnState` hook broadcasts `Frame{Type: "agent"}`, and the initial `snapshot` frame includes the current agent snapshot.
- `refreshAgent` (serialized by `credMu`) is the single place that turns credentials into capabilities: it rebuilds `tools.Deps` from the fetchers, builds a @/internal/gcal client from the stored/env Google service account (inline JSON or a file path; an invalid one is logged and leaves the calendar write tools out), and calls `agent.SetCapabilities` with `Ready = ChatGPT signed in || API key set`. It runs at startup, after every admin credential change, and from the ChatGPT client's `OnChange` (sign-in, sign-out, or a refresh that found the sign-in revoked).
- `agentToken` is the agent's per-request credential: the ChatGPT access token when signed in (refreshed by the client as needed; `chatgpt.ErrSignedOut` is wrapped as `agent.ErrSignInAgain`), otherwise the API key, otherwise `""`.
- Credential endpoints (`agent_auth.go`) for the admin Assistant section. Every success returns the `AuthStatus` JSON (`using`: `chatgpt` / `apiKey` / `none`, plus `chatgpt {signedIn, email}`, `apiKey {set, fromEnv}`, `google {set, fromEnv, serviceAccountEmail}`) and errors are `{"error": "<message for the user>"}`. Bodies are capped at 64 KiB.

| Endpoint | Behaviour |
|---|---|
| `GET /api/agent/auth` | Current `AuthStatus` |
| `POST /api/agent/auth/chatgpt/start` | `{"authorizeUrl"}` to open in a browser |
| `POST /api/agent/auth/chatgpt/complete` `{redirectUrl}` | Finishes sign-in from the pasted callback URL; any failure is 400 with the client's user-facing message |
| `DELETE /api/agent/auth/chatgpt` | Sign out; if OpenAI didn't confirm revocation but tokens were cleared, 200 with `warning` set |
| `PUT` / `DELETE /api/agent/auth/api-key` `{key}` | Save (trimmed, non-empty) or clear the API key |
| `PUT` / `DELETE /api/agent/auth/google` `{json}` | Save (validated by `gcal.ServiceAccountEmail`) or clear the service account |

  Changing a value set by an environment variable returns 409 (`secrets.ErrFromEnv`); a save failure is 500. Key/Google changes go through `credentialChanged`, which calls `refreshAgent` before answering; ChatGPT changes reach `refreshAgent` through `OnChange`.
- `POST /api/agent/converse` implements the Home Assistant "Webhook Conversation" integration contract: request `{conversation_id, query, stream}` (other fields Home Assistant sends, such as exposed entities and message history, are ignored), response `200 {"output": "<text to speak>"}`, or when `stream` is true (the integration's default) newline-delimited JSON `{"type":"item","content":"<text>"}` then `{"type":"end"}` (`writeSpoken`); the reply is produced whole, so it is one item. That integration speaks nothing on a non-200, so every answerable request is a 200 with a spoken explanation: disabled in config (`agentDisabledReply`), no credential, signed out of ChatGPT, plan usage limit, model failure or timeout (replies from the agent). Only an undecodable body, a body over 4 MiB (`http.MaxBytesReader`; Home Assistant's payload includes every exposed entity), or a blank `query` is a 400 (still with an `output`). Model errors are logged server-side. The display's optional text box uses the same endpoint.
- `GET /api/agent` returns the `AgentSnapshot` (state, heard, reply, at, `ready`, `calendarWritable`).
- `Hub` (`hub.go`): a set of clients each with a buffered `send` channel. `Broadcast` marshals once and does a non-blocking send per client; a client whose buffer is full simply misses that frame (slow clients are never allowed to stall the broadcaster).

### Things to Know

- The hot tub and pool target endpoints take an absolute target, not a delta, so a retried or reordered request is idempotent. The frontend (`useSetpoint` in @/web/src/display) relies on this when it debounces a burst of arrow taps into a single send. The assistant's set-target tools call the same fetcher `SetTarget` methods, so voice changes get identical validation and broadcasts.
- Secrets live in @/internal/secrets (`secrets.json`), never in config: `GET /api/config` and every `config` frame expose the whole config to any browser on the network without authentication. No GET endpoint or WebSocket frame returns a secret value; `AuthStatus` only carries set/fromEnv flags, the ChatGPT email and the service-account email.
- The agent is built before `applyFetcherConfig` runs and holds the fetcher pointers, not snapshots, so tools always read the latest cached data; the agent itself is never rebuilt: config saves are picked up because it re-reads model/effort each turn, and credential changes go through `refreshAgent` / `SetCapabilities`.
- Two errors from `hottub.SetTarget` (and likewise `pool.SetTarget`) look similar to a caller but are handled differently: `ErrInvalidTarget` is a client mistake (400, message passed through), anything else is logged and hidden behind a generic 502.
- `Frame` fields are all `omitempty` except `Type`. A clearing broadcast (e.g. `Frame{Type: "hottub", HotTub: nil}`) therefore serialises as just `{"type":"hottub"}`, and the frontend's `?? null` handling in @/web/src/useLiveData.ts turns the missing field back into null.
- `types.Config.Layout` is a `json.RawMessage` (`layout,omitempty`) that Go never interprets; only @/web/src/display/layout.ts understands it. It has a single writer: `Store.Replace` (used by `PUT /api/config`) always keeps the stored layout and discards the incoming one, so the admin UI, which PUTs back the whole config it loaded earlier, cannot wipe a layout saved since. Only `PUT /api/layout` changes it.
- Config saves restart fetchers with the server's root context, not the request context, so a fetcher's polling loop is not tied to the lifetime of the `PUT /api/config` request.
- Adding a widget touches a fixed set of places in this folder: a fetcher field and `New` wiring in `router.go`, a branch in `applyFetcherConfig` and `Shutdown`, a `Frame` field in `hub.go`, the initial `snapshot` frame in `ws.go`, and the get/refresh(/search) handlers. The hockey widget is the most recent example and mirrors baseball line for line; the clearing-frame test in `router_test.go` counts one frame per widget, so it changes whenever a widget is added.
- Hockey config is normalised in @/internal/config (`TeamAbbr` trimmed and upper-cased, a missing `hockey` section backfilled to enabled) before it reaches this layer, so handlers and `applyFetcherConfig` can compare `TeamAbbr` against `""` and pass it straight to the fetcher, which interpolates it into the upstream URL path.
- `handleWS` accepts connections with `InsecureSkipVerify: true` (no Origin check) because this is a homelab deployment.
- Handler tests (`handlers_test.go`) construct a partial `Server` struct directly (only the fields the handler touches) against a temp-dir config store and `httptest`. The hot tub target tests use a real `hottub.Fetcher` pointed at a closed loopback port or a malformed host string rather than mocking the fetcher; the 400 cases are rejected before any network traffic, and the malformed host produces the 502 path.

Created and maintained by Nori.

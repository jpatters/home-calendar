# Noridoc: server

Path: @/internal/server

### Overview

- The HTTP and WebSocket layer of home-calendar. It owns one `Fetcher` per widget (calendar, weather, snow day, tide, baseball, pool, hot tub), the config store, and a `Hub` that fans out live-data frames to every connected display.
- Serves a JSON API under `/api/*` for the admin UI and the display, a single `/api/ws` WebSocket for push updates, and the embedded Vite build for everything else.
- Started once from @/cmd/server/main.go via `New(ctx, store)`; `Shutdown()` stops all fetchers.

### How it fits into the larger codebase

- Every widget package under @/internal (e.g. @/internal/hottub, @/internal/tide, @/internal/pool) exposes the same fetcher contract: `New(onUpdate)`, `Start`, `Stop`, `RefreshNow`, `Snapshot`. `router.go` wires each one's `onUpdate` callback to `hub.Broadcast` with a typed `Frame`, so widget packages never know about WebSockets and the server never knows how data is fetched.
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
 internal/{ical,weather,tide,baseball,pool,hottub,snowday}.Fetcher
```

### Core Implementation

- `Server` struct (`router.go`) holds the config store, one fetcher per widget, the `Hub`, the root context used to restart fetchers, and injectable search functions (geocode, team search, tide station search) so tests can stub upstream HTTP.
- Fetcher lifecycle: `applyFetcherConfig(ctx, cfg, broadcastClears)` starts enabled fetchers with their per-widget refresh interval from `Display` config and stops disabled ones. When called from a config save (`broadcastClears=true`) it also broadcasts a nil snapshot for disabled widgets, and for tide/baseball it clears *before* starting so a stale snapshot is not relabelled with the new station/team.
- Handler conventions (`handlers.go`): `GET /api/<widget>` returns the cached snapshot or JSON `null`; `POST /api/<widget>/refresh` returns 409 when the widget is disabled, otherwise calls `RefreshNow` under a 20 s (45 s for calendars) timeout and returns `{"ok":true}`; search endpoints validate `q`, cap its length, and map upstream errors to 502.
- Hot tub write path: `POST /api/hottub/target` takes `{"targetF": <number>}`. It returns 409 if the widget is disabled, 400 when the body is not JSON or `targetF` is missing, 400 when the driver reports `hottub.ErrInvalidTarget` (fractional or outside the tub's min/max), 502 when the module fails to acknowledge, and 200 with the post-write `HotTubSnapshot` on success. The driver's `Fetcher.SetTarget` also fires `onUpdate`, so the hub broadcasts the same snapshot to all displays independently of the HTTP response.
- WebSocket (`ws.go`): on connect the client is registered with the hub and immediately sent a `snapshot` frame containing the current config plus every fetcher's cached snapshot; after that it only receives what the hub broadcasts. A reader goroutine detects disconnects, and a 30 s ping keeps the connection alive.
- `Hub` (`hub.go`): a set of clients each with a buffered `send` channel. `Broadcast` marshals once and does a non-blocking send per client; a client whose buffer is full simply misses that frame (slow clients are never allowed to stall the broadcaster).

### Things to Know

- The endpoint takes an absolute target, not a delta, so a retried or reordered request is idempotent. The frontend (@/web/src/display/HotTubModal.tsx) relies on this when it debounces a burst of arrow taps into a single send.
- Two errors from `hottub.SetTarget` look similar to a caller but are handled differently: `ErrInvalidTarget` is a client mistake (400, message passed through), anything else is logged and hidden behind a generic 502.
- `Frame` fields are all `omitempty` except `Type`. A clearing broadcast (e.g. `Frame{Type: "hottub", HotTub: nil}`) therefore serialises as just `{"type":"hottub"}`, and the frontend's `?? null` handling in @/web/src/useLiveData.ts turns the missing field back into null.
- Config saves restart fetchers with the server's root context, not the request context, so a fetcher's polling loop is not tied to the lifetime of the `PUT /api/config` request.
- `handleWS` accepts connections with `InsecureSkipVerify: true` (no Origin check) because this is a homelab deployment.
- Handler tests (`handlers_test.go`) construct a partial `Server` struct directly (only the fields the handler touches) against a temp-dir config store and `httptest`. The hot tub target tests use a real `hottub.Fetcher` pointed at a closed loopback port or a malformed host string rather than mocking the fetcher; the 400 cases are rejected before any network traffic, and the malformed host produces the 502 path.

Created and maintained by Nori.

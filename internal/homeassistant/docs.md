# Noridoc: homeassistant

Path: @/internal/homeassistant

### Overview

- Lets Ziggy see and control the household's Home Assistant devices over Home Assistant's WebSocket API (`/api/websocket`), with a long-lived access token.
- Only entities exposed to Assist (and not hidden) are visible or controllable, so the household's Assist expose list decides what Ziggy can touch. Security devices are always look-only (see below).
- `hatest` is an in-memory fake Home Assistant (handshake, `get_states`, `get_config`, the registries, `call_service` limited to each domain's real services, optional delays and failures) used by this package's tests and by @/internal/agent/tools and @/internal/server.

### How it fits into the larger codebase

```
 admin Connection > Home Assistant --PUT /api/agent/auth/home-assistant--> server (Check, then secrets)
 server.refreshAgent --> tools.Deps.Home = homeassistant.New(url, token)
 Ziggy: list_devices --> Client.Devices   control_device --> Client.Control --> call_service
```

- The URL and token live in @/internal/secrets (env `HOME_ASSISTANT_URL` / `HOME_ASSISTANT_TOKEN` override). The server checks them with `Check` before saving.
- @/internal/agent/tools wraps `Devices` and `Control` as Ziggy's `list_devices` and `control_device`.

### Core Implementation

- `New` keeps only the address's scheme and host (so a dashboard URL like `http://ha:8123/lovelace/0` works) and builds `ws(s)://host/api/websocket`.
- `Client` connects afresh for every call (dial, `auth_required` → `auth` → `auth_ok`, then commands with ids from 1 and increasing; read limit 32 MiB because a real `get_states` is often hundreds of KB), all within an 8 s timeout (or the caller's sooner deadline), and drops the connection with `CloseNow`. Dial failures are logged; an HTTP answer that isn't a WebSocket upgrade, or a WebSocket that doesn't start with `auth_required`, is "something answered at <host>, but it isn't Home Assistant"; no answer is "couldn't reach Home Assistant at <host>"; `auth_invalid` is `ErrBadToken`; command errors are "Home Assistant said: <message>".
- `load` reads `config/entity_registry/list` (exposure is `options.conversation.should_expose` and `hidden_by` null, readable with a non-admin token; `homeassistant/expose_entity/list` would need admin), `get_states`, `get_config` (temperature unit), `config/entity_registry/get_entries` (aliases) and the area and device registries (an entity without an area takes its device's). Both `Devices` and `Control` use it, so control can only act on what listing would show, with the same actions and look-only flags.
- **Look-only** (`securityDevice`, plus device sharing): locks; alarm panels; covers with device class `garage`, `gate`, `door` or none at all; switches, input booleans and buttons whose id or name mentions garage, gate, lock(s) or deadbolt; and any entity on the same Home Assistant device as one of those, exposed or not (e.g. the relay behind a garage cover).
- **Actions follow what each entity reports it supports** (`actionsFor`): lights get set_brightness only if dimmable, set_color only with an hs/xy/rgb(w)(w) colour mode, set_color_temperature only with `color_temp` (`supported_color_modes`); fans set_speed with SET_SPEED; media players on/off/play/pause/set_volume by their feature bits (volume 0–100 → `volume_level` 0–1); covers open/close/stop/set_position by feature bits; thermostats on/off/set_temperature by feature bits (range-checked against `min_temp`–`max_temp`) and set_mode when `hvac_modes` is present; switches and input booleans on/off/toggle; scenes activate; scripts run (`script.turn_on`, which doesn't wait for the script). Values are range-checked; refusals and bad input never reach Home Assistant.
- A `call_service` that outlives the deadline is reported as "didn't confirm … in time; it may still happen", not as a failure, so Ziggy doesn't retry a toggle.
- Thermostat details include target, current, min and max temperature, the unit (from `get_config`) and the modes.

### Things to Know

- The look-only rule only knows how devices are modelled. A relay for a door that isn't named for it and isn't on the same Home Assistant device as the lock/cover would still be controllable, so keep such relays unexposed. A blind without a device class is look-only; set its **Show as** in Home Assistant to make it controllable.
- Scenes and scripts exposed to Assist can be run, and either can unlock or open things the look-only rule protects. The household chose to allow scripts; keep any scene or script that touches locks, alarms or doors unexposed.
- Entities outside the entity registry (no unique id) are never exposed to Ziggy. Exposure that Home Assistant hasn't stored yet for a brand-new entity also reads as not exposed until Assist has evaluated it.
- The token has its user's permissions. A dedicated non-admin "Ziggy" user is recommended; the client never needs admin commands.

Created and maintained by Nori.

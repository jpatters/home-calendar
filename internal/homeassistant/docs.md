# Noridoc: homeassistant

Path: @/internal/homeassistant

### Overview

- Lets Ziggy see and control the household's Home Assistant devices over Home Assistant's WebSocket API (`/api/websocket`), with a long-lived access token.
- Only entities exposed to Assist are visible or controllable, so the household's existing Assist expose list is the single place that decides what Ziggy can touch. Locks, alarm panels, and covers with device class `garage` or `gate` are always look-only.
- `hatest` is an in-memory fake Home Assistant (handshake, states, registries, `call_service`) used by this package's tests and by @/internal/agent/tools and @/internal/server.

### How it fits into the larger codebase

```
 admin Connection > Home Assistant --PUT /api/agent/auth/home-assistant--> server (Check, then secrets)
 server.refreshAgent --> tools.Deps.Home = homeassistant.New(url, token)
 Ziggy: list_devices --> Client.Devices   control_device --> Client.Control --> call_service
```

- The URL and token live in @/internal/secrets (env `HOME_ASSISTANT_URL` / `HOME_ASSISTANT_TOKEN` override). The server checks them with `Check` before saving.
- @/internal/agent/tools wraps `Devices` and `Control` as Ziggy's `list_devices` and `control_device`.

### Core Implementation

- `Client` connects afresh for every call (dial, `auth_required` → `auth` → `auth_ok`, then commands with ids starting at 1 and increasing), all within an 8 s timeout, and closes. No long-lived connection, so Home Assistant restarts need no reconnect logic. `auth_invalid` is `ErrBadToken`; other failures are sentences ("couldn't reach Home Assistant at <host>", "Home Assistant said: <message>").
- Exposure is read from `config/entity_registry/list` (`options.conversation.should_expose`), which a non-admin user's token can read; `homeassistant/expose_entity/list` would require an admin token. Entities outside the entity registry are therefore never exposed to Ziggy.
- `Devices` combines that list with `config/entity_registry/get_entries` (aliases), `config/area_registry/list` and `config/device_registry/list` (an entity without an area takes its device's area) and `get_states`. Each `Device` has name (`friendly_name`, else registry name), area, aliases, state, `Details` (brightness, position, temperatures, speed, volume as percentages; unit; device class), `Actions` and `LookOnly`.
- `Control(entityID, action, value, option)` re-checks exposure and the look-only rule, then maps the action to a service through `actionsFor`: light on/off/toggle, set_brightness (`brightness_pct`), set_color (`color_name`), set_color_temperature (`color_temp_kelvin`); switch / input_boolean on/off/toggle; fan plus set_speed (`fan.set_percentage`); media_player on/off/play/pause/set_volume (0–100 → `volume_level` 0–1); cover open/close/stop/set_position; climate on/off/set_temperature/set_mode (`set_hvac_mode`); scene activate; script run (`script.turn_on`, which starts the script without waiting). Values are range-checked; refusals and bad input never reach Home Assistant.

### Things to Know

- Scripts exposed to Assist can be run; the household chose to allow them, so a script that unlocks something would get around the look-only rule. Keep such scripts unexposed.
- The token has its user's permissions. A dedicated non-admin "Ziggy" user is recommended; the client never needs admin commands.

Created and maintained by Nori.

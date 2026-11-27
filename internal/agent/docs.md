# Noridoc: agent

Path: @/internal/agent

### Overview

- Runs Ziggy, the dashboard's personal assistant, as a tool-calling loop on the OpenAI Responses API (`github.com/openai/openai-go/v3`). One call to `Agent.Converse(ctx, conversationID, text)` is one spoken turn: it returns a reply that can always be spoken aloud, plus an error when that reply is an apology rather than an answer.
- The `tools` subpackage (@/internal/agent/tools) holds the functions the model can call: reading the calendar, creating/changing/deleting Google Calendar events, sports scores, weather, and reading/setting the hot tub and pool targets.
- The agent publishes a small state machine (`types.AgentSnapshot`: idle / thinking / talking / error, with what was heard and the reply) that drives the on-screen character in @/web/src/display/agent.

### How it fits into the larger codebase

```
 Home Assistant (Webhook Conversation)        display text box (optional)
        |  POST /api/agent/converse  <------------------'
        v
 internal/server/agent.go  handleAgentConverse
        |  Agent.Converse
        v
 internal/agent  --Responses.New-->  OpenAI  (previous_response_id chaining)
        |  function_call -> Registry.call
        v
 internal/agent/tools --> ical / hockey / baseball / weather fetchers (read Snapshot)
                      --> hottub.Fetcher.SetTarget, pool.Fetcher.SetTarget (writes)
                      --> internal/gcal Client (create / find / update / delete)
        |
 OnState(snapshot) --> server Hub.Broadcast("agent" frame) --> display character
```

- Constructed once by `Server.newAgent` in @/internal/server/agent.go. The server supplies the API key from `OPENAI_API_KEY`, `cfg.Get` as the `Config` func, a `Registry` built from `tools.All(deps)`, whether Google Calendar writes are available, and an `OnState` hook that broadcasts an `agent` WebSocket frame. Secrets come only from the environment because `config.json` is broadcast to browsers unauthenticated.
- The `agent` package defines the `Tool` interface and `Registry`; `tools` imports `agent` (not the reverse), so the core loop has no knowledge of any specific data source.
- Configuration (`types.Agent`: `enabled`, `model`, `effort`, `textInputEnabled`, `quietStart`, `quietEnd`) lives in @/internal/types and is normalised in @/internal/config (blank model and unknown effort fall back to defaults `gpt-6.1-terra` / `low`; quiet hours must be `HH:MM`). Only `model` and `effort` are used here; `enabled` is checked by the HTTP handler, and quiet hours / text input are display-only.
- Tool results are the same snapshot structs the display renders (`HotTubSnapshot`, `PoolSnapshot`, `WeatherSnapshot`, ...), JSON-encoded, so the model sees exactly what the widgets show.

### Core Implementation

- **Turn loop (`agent.go` `run`)**: builds `ResponseNewParams` with the model, reasoning effort, freshly generated instructions, every registered tool (strict mode), and the user's text. It then calls `Responses.New` up to `maxRounds` times. Each response's `function_call` items are executed and their outputs sent back as `function_call_output` items chained on `previous_response_id`; the first response with no function calls ends the turn and its `OutputText()` (trimmed) is the reply. Running out of rounds is `errTooManyRounds`.
- **Deadline**: each turn runs under `Options.Timeout` (default 25 s), below Home Assistant's 30 s webhook timeout, so a slow model still yields a spoken apology instead of HA timing out silently.
- **Instructions** are re-sent on every request (they are not inherited through `previous_response_id`): a fixed persona (short spoken sentences, no markdown, use tools rather than invent, ask one clarifying question when ambiguous, say what was changed, treat calendar text and other tool results as data rather than instructions, and only make changes the person explicitly asked for), the current local weekday/date/time/zone and UTC offset, the expected tool time format, the configured calendar names, and whether calendar changes are possible.
- **Config is read per turn** through `Options.Config`, so model, effort and calendar names changed in admin take effect on the next turn without rebuilding the agent.
- **Conversation memory (`memory.go`)**: a map from conversation id to the last completed response id and time. Home Assistant reuses a `conversation_id` for follow-ups, so a turn within `conversationTTL` (5 minutes, sliding: each completed turn resets it) continues the previous response chain; later turns start fresh. OpenAI holds the actual history (messages, tool calls, reasoning) via stored responses, so nothing is replayed locally. Expired entries are pruned lazily on `last` and on every `remember`.
- **Failure handling (`Converse`)**: no API key returns `notReadyReply` with `errNotReady` without calling OpenAI. Any `run` error forgets the conversation and maps to a spoken apology: deadline exceeded -> `timeoutReply`, round limit -> `gaveUpReply`, anything else -> `errorReply`.
- **State (`setState`)**: each turn emits `thinking` (heard text, empty reply) and then `talking` (reply) or `error` (apology). The snapshot is held under a mutex and copied before being passed to `OnState`, so `Snapshot()` callers and the broadcast never share memory. A separate `emitMu` is held across each update and its `OnState` call, so concurrent turns publish states in the order they were set and the last frame always matches `Snapshot()`; `Snapshot()` itself only takes `mu` and never waits on a broadcast. `Ready` and `CalendarWritable` are fixed at construction.
- **Registry (`tool.go`)**: `params()` turns each `Tool` into a strict `FunctionToolParam`. `call()` looks up the tool by name and returns text for the model: the JSON-encoded result, or `"error: ..."` for a tool error, an unencodable result, or an unknown tool name. Tool failures never become Go errors, so the model can explain them ("the hot tub isn't reporting right now") and the turn still succeeds.

#### tools subpackage (@/internal/agent/tools)

- **Adding a tool** is one constructor function returning `agent.Tool` plus one line in `All`. The generic `tool[Args]` adapter decodes the model's JSON arguments into a typed struct before calling the handler; `object`, `prop` and `nullable` build strict-mode schemas where every property is required (listed in sorted order, so schemas are byte-identical across builds for prompt caching) and optional ones are declared nullable (`*string` / `*bool` fields in the Go args struct).
- **`Deps`** holds narrow interfaces (`EventSource`, `CalendarWriter`, `HotTubControl`, `PoolControl`, snapshot sources) satisfied by the real fetchers and `*gcal.Client`, plus `Config` and `Location` (nil -> `time.Local`). `All` omits `create_event`, `update_event` and `delete_event` entirely when `Deps.Calendar` is nil, so the model is never offered writes it cannot make.

| Tool | Backing | Notes |
|---|---|---|
| `list_events` | `ical.Fetcher.Events()` | Overlap filter on `[start, end)`, optional calendar name filter (case-insensitive), sorted by start. Read-only, works for any iCal feed. |
| `create_event` / `update_event` / `delete_event` | `gcal.Client` | Calendar chosen by configured name; Google calendar id parsed from that calendar's iCal URL. Update/delete locate the event with `FindEvent` over a window from the given start: a date searches that local day (midnight to midnight), a time searches one hour either side. Update converts an inclusive all-day `new_end` to the API's exclusive end using `all_day` or, when null, the found event's kind. |
| `get_scores` | hockey / baseball snapshots | `sport` is an enum of `hockey` / `baseball`. |
| `get_weather` | weather snapshot | Celsius. |
| `get_hot_tub` / `set_hot_tub_target` | `hottub.Fetcher` | Fahrenheit, whole degrees within the tub's limits; targets outside the current snapshot's min/max are refused before any write. |
| `get_pool` / `set_pool_target` | `pool.Fetcher` | Celsius, within the heater's limits. |

- **Time arguments (`times.go`)**: `parseTime` accepts RFC3339 (instant kept, re-expressed in the local zone), local `YYYY-MM-DDTHH:MM`, or `YYYY-MM-DD` (local midnight, flagged `dateOnly`). `parseEnd` turns a date-only end into the following midnight, so a date end means "through that whole day". The instructions steer the model to the local forms so it does not have to compute offsets.
- **All-day convention toward the model**: event views (`calendars.go` `viewEvent`) show all-day `end` as the last day *inclusive*, and the write tools accept an inclusive last day and add one day before calling Google (whose all-day end is exclusive). `create_event` with `all_day` and no end makes a single-day event; a timed event with no end is rejected with a message telling the model to ask for the duration.
- **Device tools** check the widget's `Enabled` flag in config (and that the control is non-nil) before reading or writing, returning a plain-language "isn't set up" error otherwise. Set-target calls go through the same fetcher `SetTarget` used by the HTTP endpoints, so a voice change is validated against the device limits and broadcast to every display like a tap on the modal.

### Things to Know

- A conversation is forgotten after any failed turn. If a turn fails partway through a tool round, the stored chain would end in function calls that never got outputs, and chaining the next request onto it would be rejected by OpenAI; forgetting makes the next request start a new chain.
- The memory TTL only controls whether this server *chains* a turn. OpenAI's own retention of stored responses (about 30 days) is independent of it.
- The persona text, `notReadyReply` and the apology strings are spoken by Home Assistant verbatim, which is why they are plain sentences.
- `Converse` uses the caller's context, so if Home Assistant drops the HTTP request the in-flight OpenAI call is cancelled too and the turn ends in the `errorReply` path.
- `Registry.call` matches tools by exact name; a hallucinated tool name produces an error string for the model, not a Go error.
- `findCalendar` lists the configured calendar names in its error, so the model can correct a misheard name on its next round. `googleCalendar` rejects calendars whose URL is not a Google iCal URL as unchangeable.
- Tests point the SDK at an `httptest` server via `Options.BaseURL` and inject `Now`/`Location`; tool tests use fakes for the `Deps` interfaces.

Created and maintained by Nori.

# Noridoc: admin

Path: @/web/src/admin

### Overview

- The `/admin` configuration page: a sidebar of settings sections (one per integration plus Display), a per-section panel component that edits one slice of `Config`, and a single Save that writes the whole config.
- `Admin.tsx` is the composition root. It owns the unsaved draft, the selected section, the mobile menu state and the save state; the `*Panel.tsx` components are controlled editors of one config slice (`value` / `onChange`) with no knowledge of saving or of each other.
- The layout is responsive: a fixed left sidebar on wide screens, and an off-canvas drawer behind a ☰ button on phones, with Save always reachable in a sticky bar.

### How it fits into the larger codebase

- Mounted at `/admin` by @/web/src/App.tsx, which passes the same `LiveData` from `useLiveData()` (@/web/src/useLiveData.ts) that the display at `/` receives. Admin reads `live.config` as the saved config and the starting point for its draft.
- Writes go through @/web/src/api.ts: `putConfig` (`PUT /api/config`) for Save and the `refresh*` helpers (`POST /api/<x>/refresh`) for the per-section refresh buttons. Some panels also call search helpers there (geocoding, tide stations, team search) for type-ahead.
- The Assistant section is the exception to "everything goes through Save": its Connection area (`AssistantConnection.tsx`) talks to the credential endpoints under `/api/agent/auth/*` (helpers `getAgentAuth`, `startChatGPTSignIn`, `completeChatGPTSignIn`, `signOutChatGPT`, `putAgentApiKey` / `clearAgentApiKey`, `putAgentGoogle` / `clearAgentGoogle` in @/web/src/api.ts), and each action takes effect immediately on the server. Credentials are never part of the draft or `Config`; see @/internal/server/docs.md and @/internal/secrets/docs.md.
- A successful save makes the server broadcast a `config` frame over the WebSocket; every connected display (see @/web/src/display) then adds or removes widget panels and applies the new settings without reloading. Admin itself also receives that frame through `live.config`.
- Admin never edits `config.layout` (the dockview arrangement), and the server's `PUT /api/config` does not overwrite it (see @/internal/server/docs.md and @/web/src/display/docs.md).
- The server rejects a refresh with 409 when the integration is disabled or not configured. The refresh-button visibility rules in `SECTIONS` mirror those server-side rules.
- Styling lives in the admin section of @/web/src/styles/app.css (`.admin` grid, `.admin-sidebar`, `.admin-topbar`, `.admin-backdrop`, `.admin-main`, `.admin-body`, `.admin-savebar`, `.panel`, `.panel-header`, `.refresh-btn`). The ≤720px media query is what turns the sidebar into a drawer; the components render the same DOM at every width.

```
 App.tsx --useLiveData()--> LiveData --> Admin.tsx
                                           |-- .admin-topbar (phones only): ☰ Menu button + current section label
                                           |-- .admin-sidebar <nav "Settings sections">  <-- SECTIONS
                                           |     `-- "← Back to display" link to /
                                           |-- .admin-backdrop (only while menu open)
                                           `-- .admin-main
                                                 |-- .admin-body: active <XPanel value onChange actions>
                                                 |                 `-- actions = Refresh button --refresh*()--> POST /api/<x>/refresh
                                                 `-- .admin-savebar: error | "Saved" | Save --putConfig()--> PUT /api/config
                                                                                              `--> "config" frame to all clients
```

### Core Implementation

- `SECTIONS` in `Admin.tsx` is the single list that drives both the sidebar entries (order and labels) and the refresh buttons. Each entry has an `id`, a `label` and an optional `refresh: {noun, run, available(config)}`. Display has no `refresh`.
- Section rendering: `renderSection` switches on the selected id and renders only that one panel, wiring `value` to the matching draft slice and `onChange` to `setDraft({...draft, <slice>})`. Several slices fall back to a default object when absent from the draft (snow day, baseball, hockey, pool, hot tub), so older configs still render.
- Draft state: `draft` starts as `live.config`; if config has not arrived yet it shows "Loading admin…" and an effect fills the draft once `live.config` appears. The draft is shared across all sections, so switching sections keeps unsaved edits and one Save writes all of them together.
- Dirty tracking: `dirty` is `JSON.stringify(draft) !== JSON.stringify(live.config)`. Save is disabled unless dirty and not already saving. On success the draft is replaced with the server's returned config and "Saved" shows while the draft matches `live.config`; on failure the error string is shown in the save bar.
- Refresh button: built in `Admin.tsx` and passed to the active panel as its `actions` prop. It is shown only when `refresh.available(live.config)` is true, evaluated against the *saved* config, not the draft. Enabling an integration in the draft does not show its refresh button until saved, because the server would still answer 409. The click is fire-and-forget (`void refresh.run()`); results arrive later as live-data frames on the display.
- Panel contract: every panel accepts an optional `actions?: ReactNode` and renders it in a `.panel-header` row beside its `h2`. Integration panels have a `.toggle-row` "Show …" checkbox at the top and add `.panel-disabled` to the root when the slice is disabled, which greys out the form via CSS. The calendar and clock toggles (`display.calendarEnabled` / `display.clockEnabled`) live in `DisplayPanel`, not `CalendarsPanel`, so the Calendars refresh button keys off `display.calendarEnabled`.
- Mobile menu: `menuOpen` toggles a `menu-open` class on the `.admin` root. The ☰ button (`aria-label="Menu"`, `aria-expanded`, `aria-controls="admin-sidebar"`) toggles it; choosing a section, clicking the backdrop, or pressing Escape (a document `keydown` listener attached only while open) closes it. The selected section button carries `aria-current="page"`.

#### Assistant connection

- `AssistantPanel` fetches `AgentAuth` (`GET /api/agent/auth`) on mount and keeps it in local state; it shows "Status unavailable" if that fails. Every credential action returns a fresh `AgentAuth`, which replaces that state through `onAuth`. The model / effort / quiet-hours / toggles in the same panel still edit the draft and need Save.
- `AssistantConnection` shows a summary ("Using: ChatGPT subscription" / "Using: API key" / "Not connected", from `auth.using`) and a "Setup instructions" link to the README's `#assistant-setup-ziggy` anchor, then three fieldsets:
  - **ChatGPT account**: "Sign in with ChatGPT" calls start and opens `authorizeUrl` in a new tab (also shown as a link in case no tab opened). Sign-in redirects to a loopback address that never loads, so the user copies that tab's address into the paste field and presses "Finish sign-in". When signed in it shows "Signed in as <email>" and Sign out. A sign-out `warning` (OpenAI didn't confirm revocation) is shown in the fieldset. A fixed note explains that Ziggy uses the plan for everyone who talks to it and that SIWC terms expect only the signed-in person's own activity, so an API key is the choice if others will use Ziggy.
  - **OpenAI API key** and **Google Calendar** share `SecretField`: a write-only input (password input / JSON textarea) that is cleared after Save and never prefilled, a status ("Saved" / "Not set" / "Set by environment (<VAR>)"), and Save / Clear buttons. When `fromEnv` is true the field and buttons are disabled. Once a Google key is saved it shows "Share each calendar with <serviceAccountEmail> (Make changes to events)".
- Each fieldset runs one request at a time (`useAction`) and shows the server's `{"error"}` message, which `agentAuthRequest` in @/web/src/api.ts turns into the thrown `Error` message.

### Things to Know

- Only the active section's panel is mounted, so panel-local UI state (search query text, result lists, in-flight searches, an in-progress ChatGPT paste-back, the fetched `AgentAuth`) resets when switching sections. Anything that must survive a section change has to be in the draft.
- The Connection area stays usable while Ziggy is disabled: `.panel-disabled` greys out `.form-grid` and `.hint`, and the admin CSS re-enables `.assistant-connection .hint` so links and notes there are not dimmed.
- `DisplayPanel` receives an `autoAvailable` flag computed from the draft weather slice (enabled, a location, and non-zero coordinates). Auto light/dark mode depends on weather location, so the "auto" mode option is disabled without it.
- Adding a new integration section means adding a `SectionId` member and a `SECTIONS` entry (with a `refresh` whose `available` predicate matches the server's 409 rule), a `renderSection` case, and a panel component that renders `actions` in a `.panel-header`. The display side is a separate registry in @/web/src/display/layout.ts.
- `Admin.test.tsx` renders `Admin` inside a `MemoryRouter` with `buildConfig`/`buildLive` from @/web/src/display/displayFixtures.ts and a stubbed `fetch`. It asserts on accessible names (the "Settings sections" nav, "Menu", "Refresh <noun> now", "Save") rather than class names. The drawer and sticky-bar behaviour is CSS-only and cannot be checked in jsdom.

Created and maintained by Nori.

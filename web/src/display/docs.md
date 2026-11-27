# Noridoc: display

Path: @/web/src/display

### Overview

- The kiosk-facing page of home-calendar: a calendar panel plus widget panels (clock, weather, snow day, tide, baseball, hockey, pool, hot tub) arranged in a [dockview](https://dockview.dev/) layout, the modals that open from them, and a footer bar with the live indicator and layout/refresh buttons.
- Everything renders from a single `LiveData` object; the folder holds no fetching logic of its own apart from the hot tub setpoint write and the layout save.
- `layout.ts` is the widget registry and the only place that decides which widgets are enabled; `Display.tsx` is the composition root that owns the dock, the lock/edit state and which modal is open.

### How it fits into the larger codebase

- Mounted at `/` by @/web/src/App.tsx, which owns the `useLiveData()` WebSocket hook (@/web/src/useLiveData.ts) and passes the resulting `LiveData` down. Snapshot types come from @/web/src/types.ts and mirror the Go structs in @/internal/types/types.go.
- Widgets are pure functions of props. The server pushes new snapshots over `/api/ws` (see @/internal/server), `useLiveData` folds them into state, and React re-renders; there is no polling in this folder.
- Outbound calls go through @/web/src/api.ts: `setHotTubTarget` (`POST /api/hottub/target`) and `putLayout` (`PUT /api/layout`). Neither response is applied to local state directly; the server broadcasts a `hottub` or `config` frame and the change arrives through the normal live-data path.
- The layout is persisted server-side as `config.layout`, a serialized dockview layout that the Go side treats as opaque (@/internal/config `Store.SetLayout`). Because it lives in config, every display connected to the server receives it in `config` frames. The admin UI in @/web/src/admin never touches it, and `PUT /api/config` cannot overwrite it (see @/internal/server/docs.md).
- The admin UI edits the config that gates each widget (`live.config.<widget>.enabled`); a config save produces a `config` frame that adds or removes panels here without a reload. The sports widgets additionally read the team from config (`baseball.teamId`, `hockey.teamAbbr`) to decide between a "Pick a team in admin" card and real content.
- Shared styling lives in @/web/src/styles/app.css: the `.scores-*` class family for the two score cards (only the roots `.baseball-widget` / `.hockey-widget` differ, and those are what `Display.test.tsx` queries), and the dockview CSS variables mapped onto the app palette under `.display-dock .dockview-theme-light-spaced`.
- Responsive behaviour of every panel is defined only in @/web/src/styles/app.css (a "responsive panels" section of `@container panel` rules), not in the components. Components only need to keep stable class names on the elements those rules hide.
- Uses `dockview-react` (the React binding; in v8 `dockview` is the framework-agnostic core) and its bundled stylesheet.

```
 App.tsx --useLiveData()--> LiveData --> Display.tsx
                                           |-- DisplayContext.Provider (live + modal openers)
                                           |     `-- <DockviewReact components={PANELS}>
                                           |           |-- CalendarPanel (CalendarView)
                                           |           `-- widget panels (*Widget cards, some <button onClick={onOpen}>)
                                           |-- footer .status-row: live dot | Edit layout / Reset + Done | Refresh
                                           |        `-- putLayout() --> PUT /api/layout --> "config" frame to all displays
                                           `-- *Modal components (outside the dock), gated by open-state in Display
                                                 `-- HotTubModal --setHotTubTarget()--> /api/hottub/target
```

### Core Implementation

- Widget registry (`layout.ts`): `WIDGETS` lists every panel as `{id, title, enabled(config)}` in canonical order. `enabledWidgetIds(config)` is the enabled set. Each id is used three ways: as the dockview panel id, as the component name that keys into the `PANELS` map in `Display.tsx`, and inside persisted layouts.
- Layout algorithms (`layout.ts`):

| Function | Behaviour |
|---|---|
| `reconcilePanels(api, ids)` | Removes panels whose id is not enabled; adds missing enabled ones in `WIDGETS` order. Calendar goes to the left; a widget goes below the last widget already present, or to the right of the calendar if none is. Existing panels are not moved. |
| `buildDefaultLayout(api, ids)` | `api.clear()`, then reconcile, then sizes the widget column's group to `max(300px, 25% of dock width)`, reproducing the old calendar-plus-column look. |
| `applyLayout(api, saved, ids)` | Non-object `saved` -> default. Otherwise `api.fromJSON(saved)`; if that throws (corrupt JSON shape, or a panel naming a component not in `PANELS`, which dockview rejects synchronously) -> default. On success, reconcile so the saved arrangement matches the currently enabled widgets. |

- Panel data flow: panels do not receive data via dockview panel params. `Display.tsx` provides a `DisplayContext` holding `live` and the modal openers/selectors; each `PANELS` entry reads it with `useDisplay()`. This works because dockview-react renders panels through portals inside the same React tree, so a websocket frame re-renders panels without any `updateParameters` calls.
- Dock options: `dndStrategy="pointer"` (pointer-events drag-and-drop for consistent touch behaviour; on touch a drag starts after a ~250 ms long-press), `disableFloatingGroups`, `dndEdges` set to `EDGE_DROP_ZONE` (40px activation band and drop overlay along the dock's outer edge, used to create a full-height column or full-width row; dockview's 10px default is too thin for a finger), a custom theme that spreads `themeLightSpaced` with `gap: 16` and no `colorScheme`, and a `defaultTabComponent` that is `DockviewDefaultTab` with `hideClose`.
- Panel frame: every panel, calendar and widgets alike, renders its content inside `PanelFrame` (`Display.tsx`), whose root is the `.dock-panel` element. `PanelFrame` attaches `usePreservedScroll` (`usePreservedScroll.ts`), which records `scrollTop` of any element scrolled inside the panel via a capture-phase `scroll` listener (only while the root is connected) and restores those positions from a `ResizeObserver` callback on the root. This exists because dockview re-parents a panel's DOM when the layout tree is restructured (e.g. a widget dropped onto another panel's edge), and browsers zero `scrollTop` on detached elements; without it the calendar time grid jumped from its 6am scroll position to midnight. Plain sash resizes do not move DOM and never triggered it. The hook is generic and not calendar-specific.
- Responsive panels (CSS container queries in @/web/src/styles/app.css): `.dock-panel` is `container: panel / size` with `overflow: hidden`, and `.dock-panel > .widget` fills it (full height, flex column, vertically centred, padding scaling with the panel). Type sizes use `clamp()` over container units (`cqmin`/`cqi`/`cqh`), so content grows and shrinks with the panel. Per-widget `@container panel (...)` tiers hide lower-priority details as the panel shrinks, and some widgets have wide/short variants that rearrange content side by side. All rules are scoped under `.dock-panel`, so modals and admin are unaffected. Each widget has an element that always stays visible:

| Widget | Always shown | Dropped as the panel shrinks (first to last) | Wide-panel variant |
|---|---|---|---|
| Clock | time and date (a user requirement), scaled | nothing | one line at aspect ratio >= 3 |
| Weather | icon and temperature | forecast rows one by one, feels-like meta, location, condition label | current conditions left, forecast right |
| Tide | next event | later events, location, "Now" height; narrow panels drop the High/Low word (arrow stays) and, narrower still, the height column | side-by-side layout |
| Snow day | percentage | meta, category, icon, label | side-by-side layout |
| Pool / hot tub | temperature and Heating/Idle pill | target | - |
| Baseball / hockey | Live or Latest score | meta lines (also on narrow panels), then the Next section | sections side by side |
| Calendar | the FullCalendar view | day-header weather, then toolbar compacts, then title hides and toolbar wraps (all by panel width) | - |

- The calendar's view (month/week/day) never changes with panel size; the admin default-view setting is always respected. Empty/"unavailable" states are centred in the panel.
- `CalendarView` observes its `.calendar-wrapper` with a `ResizeObserver` and calls `getApi().updateSize()`. FullCalendar only re-measures on window resize, so without this a panel resize left the calendar rendered at its old width.
- Lock/edit state: `editing` (default false) lives in `Display.tsx` and is mirrored into `editingRef` for dockview callbacks. `setLocked(api, locked)` calls `api.updateOptions({locked, disableDnd})` and sets every group's `header.hidden`, so tabs are invisible while locked. An `onDidAddGroup` handler gives groups created later (by drag or reconcile) the current header state. `locked` also disables every split view, and dockview's own CSS then makes the sashes inert, so nothing can be resized while locked.
- Save/reset: "Done" sends `api.toJSON()` via `putLayout` and locks on success. "Reset layout" rebuilds the default immediately, sends `null` (server clears the layout) and locks. A failed save leaves edit mode on and shows "Couldn't save layout" with `role="alert"` in the footer.
- Applying server pushes: an effect keyed on the enabled-id string and the JSON string of `config.layout` runs on each relevant config frame. `appliedLayoutRef` holds the JSON of the last layout applied (or saved). If locked and the pushed layout differs, it is applied with `applyLayout`, so multiple displays follow each other; the echo of this display's own save matches and is skipped. While editing, pushed layouts are ignored and only `reconcilePanels` runs, so the local edit wins when Done is pressed.
- Widget/modal convention: a widget that has detail to show renders its root as `<button type="button" className="widget ..." aria-label="<Thing> details" onClick={onOpen}>` (TideWidget, WeatherWidget, HotTubWidget). `Display.tsx` holds a boolean `xOpen` state per modal and renders the modal only while both the state is true *and* the snapshot is non-null, so a modal closes itself if its data disappears (e.g. widget disabled, hot tub goes unavailable). Widgets with nothing to open (pool, baseball, hockey, snow day, clock) are plain `div`s.
- Modal structure is shared: a `.modal-backdrop` that closes on click, wrapping a `.modal` with `role="dialog"`, an `aria-label`, `stopPropagation` on click, a `.modal-header` with an `h2` and a `.close-btn`, and a `.modal-body`. Styles live in @/web/src/styles/app.css. Weather and tide modals page through days with `useSwipe`.
- Empty states: each widget shows a `<Thing> unavailable` card when its snapshot is null, so a panel never goes blank while a fetcher is failing.
- Score cards (`BaseballWidget.tsx`, `HockeyWidget.tsx`): both render the same three-state card from a `{liveGame, latestGame, nextGame}` snapshot. While `liveGame` is set a LIVE section (pulsing dot, score line, in-game state, venue) *replaces* the Latest section; otherwise Latest shows the last final and Next shows the upcoming opponent, `formatGameTime` and venue. Each section has its own empty text ("No recent game" / "No upcoming game") so a snapshot with only one side still lays out. The sport-specific part is the in-game and final labels: baseball shows inning half and outs via `formatInning` (`baseballFormat.ts`); hockey shows `formatPeriod` ("1st".."3rd", "OT", "2OT", "SO") plus either the clock or "Intermission", and labels finals as "Final", "Final/OT" or "Final/SO" via `formatFinal` (`hockeyFormat.ts`).
- Formatting shared across sports lives in `gameFormat.ts`: `formatGameTime` (Today / Tomorrow / weekday-and-date, always with a local time) and the `ordinal` helper used by both inning and period labels.
- Hot tub read path: `HotTubWidget` shows water temperature, a Heating/Idle pill and the target, all formatted by `formatTemp` in `hotTubFormat.ts` (rounded whole degrees with a `°F` suffix).
- Hot tub write path (`HotTubModal.tsx`): the modal keeps a local `chosen` target and an `error` string. `adjust(±1)` clamps to the snapshot's `minTargetF`/`maxTargetF`, shows the new value immediately, and (re)starts a 600 ms timer. When the timer fires, `send` posts the absolute target once via `setHotTubTarget`; if the chosen value equals the live target (taps cancelled out) nothing is sent and `chosen` is cleared. On a failed request the display reverts to the live value and an error line with `role="alert"` appears. Closing the modal while a timer is pending flushes the send first. Arrow buttons are disabled at the limits.

### Things to Know

- `chosen` is reset whenever the rounded live `targetF` changes (an effect keyed on `liveTarget`). Because a stale 30 s poll re-delivers the *same* target, it does not clobber a pending tap, but a genuine change reported by the tub always wins over the local value.
- A burst of taps is one UDP write. The 600 ms debounce plus the absolute-target API (documented in @/internal/server/docs.md) is what makes rapid ▲▲▲ safe: only the final value is sent, and resending it is harmless.
- The hot tub's `enabled` predicate in `WIDGETS` defaults to `false` when config is missing, unlike the other widgets (hockey included) which default to enabled.
- Snow day counts as enabled only when it is enabled *and* has a URL. The widget renders nothing without a URL; in the old CSS grid that took no space, but as a dock panel it would be an empty box.
- Panel ids are persisted inside saved layouts. Renaming an id makes `fromJSON` throw on every stored layout that contains it, which falls back to the default layout.
- Tabs have no close button because an enabled widget is always re-added by `reconcilePanels`; closing one would only hide it until the next reload or config frame. Removing a widget is done by disabling it in admin.
- Inactive tabs in a tabbed group are unmounted (dockview's default `onlyWhenVisible` renderer), so any widget-local state is reset when its tab is re-selected.
- Widgets stacked in one column split its height equally by default. This is fine on short screens because each widget adapts to its panel via the responsive tiers rather than being clipped.
- New widgets should get their own tier rules in the responsive-panels section of @/web/src/styles/app.css (decide which element always stays visible and what drops first) and be checked across a range of panel sizes in a browser. jsdom cannot evaluate container queries, so none of the responsive behaviour is covered by tests; it was verified visually and with a scripted sweep of every widget across small-to-large panel sizes checking that nothing overflows its panel, using sample data (live-game states and unusually long names were not in that sample).
- Adding a widget on the display side means adding a `WIDGETS` entry (id, title, enabled predicate) in `layout.ts`, a `WidgetId` union member, and a matching `PANELS` component in `Display.tsx` (via the `panel(...)` helper, which wraps it in `PanelFrame`).
- The hockey snapshot carries `clock` and `inIntermission` only for a live game, and the backend fills them from a second NHL endpoint that can fail independently of the score; `HockeyWidget` therefore treats an empty clock as "render nothing" rather than as an error, and the period label alone is still shown. The `teamAbbr` shown on the card is taken from the snapshot first and falls back to config.
- Modal open state is in `Display.tsx`, not in the widget, so a widget never needs to know whether its modal exists. Adding a modal to another widget means adding an `xOpen` state and a gated render in `Display.tsx`, nothing else.
- Temperatures are formatted in the frontend from raw Fahrenheit numbers; the hot tub snapshot is Fahrenheit end-to-end (the driver converts from the module's tenths-above-freezing encoding), whereas the pool snapshot is Celsius.
- Tests for this folder use React Testing Library with fake timers where debouncing matters (`HotTubModal.test.tsx`) and assert on `aria-label`s (`Hot tub details`, `Raise target`, `Lower target`, `Close`) rather than class names.
- Dockview panels render at 0x0 in jsdom, so layout sizing and drag/resize are not testable there (they were verified manually in a browser). @/web/src/test/setup.ts stubs `ResizeObserver` for dockview, and `displayFixtures.ts` provides shared `buildConfig`/`buildLive` builders (including a `layout` override) for `Display.test.tsx` and `DisplayLayout.test.tsx`; the latter covers lock/edit, save/reset and pushed-layout behaviour.

Created and maintained by Nori.

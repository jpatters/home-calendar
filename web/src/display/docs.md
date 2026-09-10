# Noridoc: display

Path: @/web/src/display

### Overview

- The kiosk-facing page of home-calendar: a calendar pane plus a column of widgets (clock, weather, snow day, tide, baseball, pool, hot tub) and the modals that open from them.
- Everything renders from a single `LiveData` object; the folder holds no fetching logic of its own apart from the hot tub setpoint write.
- `Display.tsx` is the composition root and the only place that decides which widgets are enabled and which modal is open.

### How it fits into the larger codebase

- Mounted at `/` by @/web/src/App.tsx, which owns the `useLiveData()` WebSocket hook (@/web/src/useLiveData.ts) and passes the resulting `LiveData` down. Snapshot types come from @/web/src/types.ts and mirror the Go structs in @/internal/types/types.go.
- Widgets are pure functions of props. The server pushes new snapshots over `/api/ws` (see @/internal/server), `useLiveData` folds them into state, and React re-renders; there is no polling in this folder.
- The one outbound call is `setHotTubTarget` in @/web/src/api.ts, which hits `POST /api/hottub/target`. The response is not applied to local state directly; the server broadcasts the fresh reading as a `hottub` frame and it arrives through the normal live-data path.
- The admin UI in @/web/src/admin edits the config that gates each widget (`live.config.<widget>.enabled`); a config save produces a `config` frame that flips widgets on and off here without a reload.

```
 App.tsx --useLiveData()--> LiveData --> Display.tsx
                                           |-- CalendarView (events, weather strip)
                                           |-- *Widget components (read-only cards)
                                           |     `-- some are <button onClick={onOpen}>
                                           `-- *Modal components, gated by open-state in Display
                                                 `-- HotTubModal --setHotTubTarget()--> /api/hottub/target
```

### Core Implementation

- Widget/modal convention: a widget that has detail to show renders its root as `<button type="button" className="widget ..." aria-label="<Thing> details" onClick={onOpen}>` (TideWidget, WeatherWidget, HotTubWidget). `Display.tsx` holds a boolean `xOpen` state per modal and renders the modal only while both the state is true *and* the snapshot is non-null, so a modal closes itself if its data disappears (e.g. widget disabled, hot tub goes unavailable). Widgets with nothing to open (pool, baseball, snow day, clock) are plain `div`s.
- Modal structure is shared: a `.modal-backdrop` that closes on click, wrapping a `.modal` with `role="dialog"`, an `aria-label`, `stopPropagation` on click, a `.modal-header` with an `h2` and a `.close-btn`, and a `.modal-body`. Styles live in @/web/src/styles/app.css. Weather and tide modals page through days with `useSwipe`.
- Empty states: each widget shows a `<Thing> unavailable` card when its snapshot is null, so the layout never collapses while a fetcher is failing.
- Hot tub read path: `HotTubWidget` shows water temperature, a Heating/Idle pill and the target, all formatted by `formatTemp` in `hotTubFormat.ts` (rounded whole degrees with a `°F` suffix).
- Hot tub write path (`HotTubModal.tsx`): the modal keeps a local `chosen` target and an `error` string. `adjust(±1)` clamps to the snapshot's `minTargetF`/`maxTargetF`, shows the new value immediately, and (re)starts a 600 ms timer. When the timer fires, `send` posts the absolute target once via `setHotTubTarget`; if the chosen value equals the live target (taps cancelled out) nothing is sent and `chosen` is cleared. On a failed request the display reverts to the live value and an error line with `role="alert"` appears. Closing the modal while a timer is pending flushes the send first. Arrow buttons are disabled at the limits.

### Things to Know

- `chosen` is reset whenever the rounded live `targetF` changes (an effect keyed on `liveTarget`). Because a stale 30 s poll re-delivers the *same* target, it does not clobber a pending tap, but a genuine change reported by the tub always wins over the local value.
- A burst of taps is one UDP write. The 600 ms debounce plus the absolute-target API (documented in @/internal/server/docs.md) is what makes rapid ▲▲▲ safe: only the final value is sent, and resending it is harmless.
- `HotTubWidget` gets an `onOpen` prop but is only rendered when `live.config.hotTub.enabled` is true; the default when config is missing is `false`, unlike the other widgets which default to enabled.
- Modal open state is in `Display.tsx`, not in the widget, so a widget never needs to know whether its modal exists. Adding a modal to another widget means adding an `xOpen` state and a gated render in `Display.tsx`, nothing else.
- Temperatures are formatted in the frontend from raw Fahrenheit numbers; the hot tub snapshot is Fahrenheit end-to-end (the driver converts from the module's tenths-above-freezing encoding), whereas the pool snapshot is Celsius.
- Tests for this folder use React Testing Library with fake timers where debouncing matters (`HotTubModal.test.tsx`) and assert on `aria-label`s (`Hot tub details`, `Raise target`, `Lower target`, `Close`) rather than class names.

Created and maintained by Nori.

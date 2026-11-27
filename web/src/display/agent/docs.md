# Noridoc: agent (display)

Path: @/web/src/display/agent

### Overview

- The on-screen body of Ziggy, the assistant: an 8-bit character living in a pixel-art room, drawn entirely as SVG rectangles from character grids and room geometry in code (no image assets).
- The character's pose follows the assistant's live state (idle, thinking, looking out the window while checking the weather, talking, confused after an error, asleep during quiet hours); its outfit follows the season and Canadian holidays; the room's window shows the time of day and current weather.
- Optionally lets people type to Ziggy on the display, posting to the same endpoint Home Assistant uses.

### How it fits into the larger codebase

- Rendered as the `agent` dock panel (titled "Ziggy") by @/web/src/display/Display.tsx, enabled by `config.agent.enabled` in @/web/src/display/layout.ts. It receives `live.agent` (`AgentSnapshot`), `live.weather`, and the display-only settings `textInputEnabled` / `quietStart` / `quietEnd` from `config.agent`.
- `live.agent` arrives over the WebSocket as `agent` frames broadcast by the Go agent's `OnState` hook (@/internal/agent, @/internal/server). The panel never polls and never learns the reply from the HTTP response; voice turns from Home Assistant and typed turns both show up through the same frames.
- Typed messages go through `agentApi.converse` -> `POST /api/agent/converse` with `{conversation_id, query}`. The conversation id is created and held by `Display.tsx`, not here, because dockview remounts panel components when they are dragged; keeping it above the dock lets a follow-up continue the same server-side conversation.
- Weather data (sunrise/sunset per day and the current WMO `weatherCode`) comes from the weather widget's snapshot (@/internal/weather); without weather the scene falls back to fixed day/night hours and a clear window.

### Core Implementation

- **`scene.ts` `sceneFor({now, weather, agentState, tool, quietStart, quietEnd})`** is a pure function returning `{pose, outfit, sky, conditions}`; all date/weather rules live here and are what the tests cover.
  - Pose: `thinking` -> think, or lookout while the snapshot's `tool` is `get_weather`; `talking` -> talk, `error` -> confused; otherwise sleep inside quiet hours, else idle. Quiet hours may wrap midnight; equal start and end means never quiet. An active conversation always wakes Ziggy.
  - Outfit: a holiday outfit if a holiday falls today or within the next two days (`HOLIDAY_LEAD_DAYS`), else the season by month (Dec-Feb winter, Mar-May spring, Jun-Aug summer, Sep-Nov fall).
  - Holiday dates are computed in `scene.ts` with no library: fixed dates (Jan 1, Feb 14, Mar 17, Jul 1, Oct 31, Dec 25), Easter Sunday via the anonymous Gregorian (Meeus/Jones/Butcher) algorithm, and Canadian Thanksgiving as the second Monday of October. The nearest upcoming holiday within the lead window wins. Results are cached per year.
  - Sky: dusk within 30 minutes of today's sunrise or sunset, day between them, night otherwise; with no matching weather day, day is 07:00-19:00. Conditions map the current WMO code to partly (2) / overcast (3) / fog / rain / snow / thunder, and everything else to none.
- **`sprites/character.ts`**: the character is a fixed-size character grid (one character per pixel, `.` transparent, with headroom rows for hats). `composeCharacter(outfit, face)` copies the base body, overlays a face layer (the `back` face covers the head with hair for the lookout pose and drops front-of-shirt marks), then the outfit's marks (scarf, heart, leaf, ...) and hat, and returns the grid plus a palette where the shirt/pants/shoes letters are swapped per outfit. Outfits are therefore palette swaps plus overlays, not separate drawings.
- **`sprites/room.ts` `roomRects({sky, conditions, tick, blanket})`** returns the room as a list of rectangles (wall, window with gradient sky, sun/moon/stars, clouds and animated rain/snow/fog/lightning; overcast, rain, snow, fog and thunder grey the sky and hide the sun, moon and stars, while partly cloudy draws clouds over them; wall calendar, floor, rug, bed). Star and raindrop positions come from a seeded PRNG so they are stable across renders. It also exports the positions the panel needs: standing row, wander range and the sleeping head position on the pillow.
- **`PixelSprite.tsx`** renders a grid as run-length `<rect>`s with `shapeRendering="crispEdges"`.
- **`AgentPanel.tsx`** composes everything:
  - Timers: a 200 ms animation frame (talk mouth flapping, periodic blink, precipitation), a 60 s tick to re-evaluate the scene (quiet hours, sky, outfit), and while idle a 4 s random wander across the floor (CSS transitions the move).
  - Speech duration: a `talking` or `error` snapshot keeps its bubble for `reply.length * 60 ms`, clamped to 2-12 s, measured from the snapshot's server `at` timestamp. After that the panel treats the state as idle locally (a one-shot timeout re-renders at the end) even though the server snapshot still says talking/error. If `onSend` rejects (the server could not be reached), the panel shows a local error speech, "I couldn't reach the server. Try again.", with the same confused pose and duration rule; a newer server snapshot takes precedence over it.
  - Lookout: Ziggy hurries to `LOOKOUT_X` under the window (a 0.8 s `.to-window` transition instead of the 3 s wander) and faces away. He stays there, facing the room, while giving that turn's answer (talking or confused) and wanders off once the panel is idle again.
  - Bubble: `…` while thinking (none while looking out); the reply while talking or confused (so error apologies are shown as well as spoken). The bubble is positioned inside the room container.
  - Sleeping draws only the top rows of the character (`SLEEP_VISIBLE_ROWS`) at the pillow position, with a "z z z" overlay; confused adds a "?".
  - Text input: when `textInputEnabled`, the character is a button ("Talk to assistant") that reveals an input; Enter or Send clears it and calls `onSend`. Send errors are swallowed, since the outcome arrives as an `agent` frame.

### Things to Know

- Because the bubble timer is based on the server's `at`, a display that connects (or reloads) after a reply has expired shows Ziggy idle rather than replaying the bubble.
- The character `aria-label` (`Assistant is idle|thinking|looking out the window|talking|sleeping|confused`) is the stable hook tests use to assert pose.
- The Christmas outfit also changes the bed blanket colour; that is the only outfit-driven change to the room.
- `scene.ts` uses the browser's local time zone for dates, quiet hours and holiday matching; it is not driven by the weather location's zone.

Created and maintained by Nori.

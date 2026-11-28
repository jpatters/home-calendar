import { afterEach, expect, test } from "vitest";
import { createDockview, type DockviewApi } from "dockview-react";
import { applyLayout, buildDefaultLayout, DOCK_THEME, serializeLayout, type WidgetId } from "./layout";

const IDS: WidgetId[] = ["calendar", "clock", "weather", "tide", "pool"];
const docks: DockviewApi[] = [];

afterEach(() => {
  for (const api of docks.splice(0)) api.dispose();
  document.body.innerHTML = "";
});

// A real dockview laid out at a fixed size (jsdom has no layout engine, so
// the size is given explicitly, as the kiosk's window would give it).
function makeDock(): DockviewApi {
  const el = document.createElement("div");
  document.body.appendChild(el);
  const api = createDockview(el, {
    theme: DOCK_THEME,
    createComponent: () => ({ element: document.createElement("div"), init: () => {} }),
  });
  api.layout(1228, 565);
  docks.push(api);
  return api;
}

function sizes(api: DockviewApi) {
  return Object.fromEntries(
    api.panels.map((p) => [p.id, [Math.round(p.group.width), Math.round(p.group.height)]]),
  );
}

test("a saved layout loads back at the sizes it was saved with", () => {
  const original = makeDock();
  buildDefaultLayout(original, IDS);
  original.getPanel("weather")!.group.api.setSize({ height: 180 });
  original.getPanel("calendar")!.group.api.setSize({ width: 800 });
  const saved = JSON.parse(JSON.stringify(serializeLayout(original)));

  const restored = makeDock();
  applyLayout(restored, saved, IDS);

  expect(sizes(restored)).toEqual(sizes(original));
});

test("a layout with a split inside the widget column loads back at its saved sizes", () => {
  const original = makeDock();
  buildDefaultLayout(original, IDS);
  original.getPanel("tide")!.api.moveTo({ group: original.getPanel("weather")!.group, position: "right" });
  original.getPanel("weather")!.group.api.setSize({ width: 120 });
  const saved = JSON.parse(JSON.stringify(serializeLayout(original)));

  const restored = makeDock();
  applyLayout(restored, saved, IDS);

  expect(sizes(restored)).toEqual(sizes(original));
});

test("saving and loading repeatedly does not drift", () => {
  let dock = makeDock();
  buildDefaultLayout(dock, IDS);
  dock.getPanel("weather")!.group.api.setSize({ height: 200 });
  const first = sizes(dock);

  for (let i = 0; i < 5; i++) {
    const saved = JSON.parse(JSON.stringify(serializeLayout(dock)));
    dock = makeDock();
    applyLayout(dock, saved, IDS);
  }

  expect(sizes(dock)).toEqual(first);
});

import { themeLightSpaced, type DockviewApi, type DockviewTheme, type SerializedDockview } from "dockview-react";
import type { Config } from "../types";

// Panel ids double as dockview component names and are persisted in saved
// layouts, so renaming one orphans it in every stored layout.
export type WidgetId =
  | "calendar"
  | "clock"
  | "weather"
  | "snowday"
  | "tide"
  | "baseball"
  | "hockey"
  | "pool"
  | "hottub";

interface WidgetDef {
  id: WidgetId;
  title: string;
  enabled: (c: Config | null) => boolean;
}

// Canonical order: the default layout stacks widgets in this order, and a
// widget missing from a saved layout is appended below the last one present.
export const WIDGETS: WidgetDef[] = [
  { id: "calendar", title: "Calendar", enabled: (c) => c?.display.calendarEnabled ?? true },
  { id: "clock", title: "Clock", enabled: (c) => c?.display.clockEnabled ?? true },
  { id: "weather", title: "Weather", enabled: (c) => c?.weather.enabled ?? true },
  // Without a URL the widget renders nothing, so it gets no (empty) panel.
  { id: "snowday", title: "Snow day", enabled: (c) => (c?.snowDay.enabled ?? true) && !!c?.snowDay.url },
  { id: "tide", title: "Tide", enabled: (c) => c?.tide.enabled ?? true },
  { id: "baseball", title: "Baseball", enabled: (c) => c?.baseball.enabled ?? true },
  { id: "hockey", title: "Hockey", enabled: (c) => c?.hockey.enabled ?? true },
  { id: "pool", title: "Pool", enabled: (c) => c?.pool.enabled ?? true },
  { id: "hottub", title: "Hot tub", enabled: (c) => c?.hotTub.enabled ?? false },
];

export const DOCK_THEME: DockviewTheme = {
  ...themeLightSpaced,
  name: "home",
  gap: 16,
  colorScheme: undefined,
};

const WIDGET_COLUMN_FRACTION = 0.25;
const WIDGET_COLUMN_MIN_PX = 300;

export function enabledWidgetIds(config: Config | null): WidgetId[] {
  return WIDGETS.filter((w) => w.enabled(config)).map((w) => w.id);
}

// Adds panels for enabled widgets that are missing and removes panels for
// widgets that are not enabled, leaving everything else where it is.
export function reconcilePanels(api: DockviewApi, ids: WidgetId[]): void {
  const wanted = new Set<string>(ids);
  for (const panel of [...api.panels]) {
    if (!wanted.has(panel.id)) api.removePanel(panel);
  }
  for (const def of WIDGETS) {
    if (!wanted.has(def.id) || api.getPanel(def.id)) continue;
    api.addPanel({
      id: def.id,
      component: def.id,
      title: def.title,
      position: positionFor(api, def.id),
    });
  }
}

function positionFor(api: DockviewApi, id: WidgetId) {
  if (api.panels.length === 0) return undefined;
  if (id === "calendar") return { direction: "left" as const };
  const lastWidget = WIDGETS.filter(
    (w) => w.id !== "calendar" && api.getPanel(w.id),
  ).pop();
  if (lastWidget) return { referencePanel: lastWidget.id, direction: "below" as const };
  return { referencePanel: "calendar", direction: "right" as const };
}

// Calendar on the left, the widgets stacked in a narrow column on the right.
export function buildDefaultLayout(api: DockviewApi, ids: WidgetId[]): void {
  api.clear();
  reconcilePanels(api, ids);
  const firstWidget = ids.find((id) => id !== "calendar");
  if (!firstWidget || !ids.includes("calendar")) return;
  const width = Math.max(WIDGET_COLUMN_MIN_PX, api.width * WIDGET_COLUMN_FRACTION);
  api.getPanel(firstWidget)?.group.api.setSize({ width });
}

export function applyLayout(api: DockviewApi, saved: unknown, ids: WidgetId[]): void {
  if (saved === null || typeof saved !== "object") {
    buildDefaultLayout(api, ids);
    return;
  }
  try {
    api.fromJSON(saved as SerializedDockview);
  } catch (err) {
    // A layout that no longer matches the registered widgets (or is simply
    // corrupt) must not leave the kiosk blank.
    console.warn("saved layout rejected, using the default layout", err);
    buildDefaultLayout(api, ids);
    return;
  }
  reconcilePanels(api, ids);
}

type SerializedNode = SerializedDockview["grid"]["root"];

// Dockview (8.10) saves each panel's on-screen size, which is already reduced
// by its share of the gaps, but on load it treats the saved sizes as the full
// sizes the gaps are still taken out of. Left alone, every save/load cycle
// shrinks the total and rescales it, so larger panels creep larger. Adding
// each panel's gap share back makes the saved layout load at its saved size.
export function serializeLayout(api: DockviewApi): SerializedDockview {
  const layout = api.toJSON();
  return { ...layout, grid: { ...layout.grid, root: restoreGapShares(layout.grid.root) } };
}

function restoreGapShares(node: SerializedNode): SerializedNode {
  if (node.type !== "branch") return node;
  const children = node.data as SerializedNode[];
  const visible = children.filter((c) => c.visible !== false).length;
  const share = visible > 1 ? (DOCK_THEME.gap ?? 0) * (visible - 1) / visible : 0;
  return {
    ...node,
    data: children.map((child) => {
      const restored = restoreGapShares(child);
      if (child.visible === false || restored.size === undefined) return restored;
      return { ...restored, size: restored.size + share };
    }),
  };
}

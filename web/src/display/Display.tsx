import { createContext, useContext, useEffect, useRef, useState, type ReactNode } from "react";
import {
  DockviewDefaultTab,
  DockviewReact,
  type DockviewApi,
  type DockviewReadyEvent,
  type IDockviewPanelHeaderProps,
  type IDockviewPanelProps,
} from "dockview-react";
import "dockview-react/dist/styles/dockview.css";
import type { CalendarEvent } from "../types";
import type { LiveData } from "../useLiveData";
import CalendarView from "./CalendarView";
import ClockWidget from "./ClockWidget";
import WeatherWidget from "./WeatherWidget";
import WeatherModal from "./WeatherModal";
import SnowDayWidget from "./SnowDayWidget";
import TideWidget from "./TideWidget";
import TideModal from "./TideModal";
import BaseballWidget from "./BaseballWidget";
import HockeyWidget from "./HockeyWidget";
import PoolWidget from "./PoolWidget";
import HotTubWidget from "./HotTubWidget";
import HotTubModal from "./HotTubModal";
import EventModal from "./EventModal";
import DayModal from "./DayModal";
import { reloadPage } from "../browser";
import { putLayout } from "../api";
import { usePreservedScroll } from "./usePreservedScroll";
import {
  applyLayout,
  buildDefaultLayout,
  DOCK_THEME,
  enabledWidgetIds,
  reconcilePanels,
  serializeLayout,
  type WidgetId,
} from "./layout";

interface Props {
  live: LiveData;
}

// Panels render through dockview portals, which still sit inside this React
// tree, so they read live data from context instead of panel params.
interface DisplayContextValue {
  live: LiveData;
  openWeather: () => void;
  openTide: () => void;
  openHotTub: () => void;
  selectEvent: (ev: CalendarEvent) => void;
  selectDay: (dayISO: string) => void;
}

const DisplayContext = createContext<DisplayContextValue | null>(null);

function useDisplay(): DisplayContextValue {
  const ctx = useContext(DisplayContext);
  if (!ctx) throw new Error("display panel rendered outside DisplayContext");
  return ctx;
}

function PanelFrame({ children }: { children: ReactNode }) {
  const ref = useRef<HTMLDivElement>(null);
  usePreservedScroll(ref);
  return (
    <div className="dock-panel" ref={ref}>
      {children}
    </div>
  );
}

function CalendarPanel() {
  const { live, selectEvent, selectDay } = useDisplay();
  return (
    <PanelFrame>
      <div className="calendar-pane">
      <CalendarView
        events={live.events}
        defaultView={live.config?.display.defaultView ?? "week"}
        onEventClick={selectEvent}
        onDayClick={selectDay}
        weather={live.weather}
        weatherConfig={live.config?.weather}
        />
      </div>
    </PanelFrame>
  );
}

function panel(render: (ctx: DisplayContextValue) => JSX.Element) {
  return function WidgetPanel(_: IDockviewPanelProps) {
    return <PanelFrame>{render(useDisplay())}</PanelFrame>;
  };
}

const PANELS: Record<WidgetId, React.FunctionComponent<IDockviewPanelProps>> = {
  calendar: CalendarPanel,
  clock: panel(() => <ClockWidget />),
  weather: panel(({ live, openWeather }) => (
    <WeatherWidget weather={live.weather} config={live.config?.weather} onOpen={openWeather} />
  )),
  snowday: panel(({ live }) => (
    <SnowDayWidget snowday={live.snowday} config={live.config?.snowDay} />
  )),
  tide: panel(({ live, openTide }) => (
    <TideWidget tide={live.tide} config={live.config?.tide} onOpen={openTide} />
  )),
  baseball: panel(({ live }) => (
    <BaseballWidget baseball={live.baseball} config={live.config?.baseball} />
  )),
  hockey: panel(({ live }) => <HockeyWidget hockey={live.hockey} config={live.config?.hockey} />),
  pool: panel(({ live }) => <PoolWidget pool={live.pool} />),
  hottub: panel(({ live, openHotTub }) => (
    <HotTubWidget hottub={live.hottub} onOpen={openHotTub} />
  )),
};

// Enabled widgets always have a panel, so a close button would only make a
// widget vanish until panels are next reconciled.
function Tab(props: IDockviewPanelHeaderProps) {
  return <DockviewDefaultTab {...props} hideClose />;
}

// The default 10px band along the dock's outer edge is too thin for a finger.
const EDGE_DROP_ZONE = {
  activationSize: { type: "pixels" as const, value: 40 },
  size: { type: "pixels" as const, value: 40 },
};

function setLocked(api: DockviewApi, locked: boolean): void {
  api.updateOptions({ locked, disableDnd: locked });
  for (const group of api.groups) group.header.hidden = locked;
}

export default function Display({ live }: Props) {
  const [selectedEvent, setSelectedEvent] = useState<CalendarEvent | null>(null);
  const [selectedDay, setSelectedDay] = useState<string | null>(null);
  const [tideOpen, setTideOpen] = useState(false);
  const [weatherOpen, setWeatherOpen] = useState(false);
  const [hotTubOpen, setHotTubOpen] = useState(false);
  const [editing, setEditing] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);

  const apiRef = useRef<DockviewApi | null>(null);
  const editingRef = useRef(editing);
  // JSON of the saved layout last applied to the dock, so a config frame that
  // carries the same layout (e.g. the echo of our own save) is not re-applied.
  const appliedLayoutRef = useRef<string | null>(null);

  const ids = enabledWidgetIds(live.config);
  const idsKey = ids.join(",");
  const savedLayout = live.config?.layout ?? null;
  const layoutKey = JSON.stringify(savedLayout);

  useEffect(() => {
    editingRef.current = editing;
    const api = apiRef.current;
    if (api) setLocked(api, !editing);
  }, [editing]);

  useEffect(() => {
    const api = apiRef.current;
    if (!api) return;
    if (!editingRef.current && layoutKey !== appliedLayoutRef.current) {
      applyLayout(api, savedLayout, ids);
      appliedLayoutRef.current = layoutKey;
    } else {
      reconcilePanels(api, ids);
    }
    setLocked(api, !editingRef.current);
    // ids and savedLayout are tracked through their string keys.
  }, [layoutKey, idsKey]);

  if (!live.ready) {
    return <div className="loading">Loading…</div>;
  }

  const onReady = (event: DockviewReadyEvent) => {
    const api = event.api;
    apiRef.current = api;
    applyLayout(api, savedLayout, ids);
    appliedLayoutRef.current = layoutKey;
    setLocked(api, !editingRef.current);
    // Groups created by dragging or reconciling start with a visible header.
    api.onDidAddGroup((group) => {
      group.header.hidden = !editingRef.current;
    });
  };

  const save = async (layout: object | null) => {
    setSaveError(null);
    try {
      await putLayout(layout);
      appliedLayoutRef.current = JSON.stringify(layout);
      setEditing(false);
    } catch {
      setSaveError("Couldn't save layout");
    }
  };

  const done = () => {
    const api = apiRef.current;
    if (api) void save(serializeLayout(api));
  };

  const reset = () => {
    const api = apiRef.current;
    if (!api) return;
    buildDefaultLayout(api, ids);
    void save(null);
  };

  const ctx: DisplayContextValue = {
    live,
    openWeather: () => setWeatherOpen(true),
    openTide: () => setTideOpen(true),
    openHotTub: () => setHotTubOpen(true),
    selectEvent: setSelectedEvent,
    selectDay: setSelectedDay,
  };

  return (
    <DisplayContext.Provider value={ctx}>
      <div className={editing ? "display editing" : "display"}>
        <div className="display-dock">
          <DockviewReact
            components={PANELS}
            defaultTabComponent={Tab}
            onReady={onReady}
            theme={DOCK_THEME}
            dndStrategy="pointer"
            disableFloatingGroups
            dndEdges={EDGE_DROP_ZONE}
          />
        </div>

        <footer className="status-row">
          <div className="connection-indicator">
            <span className={live.connected ? "dot ok" : "dot bad"} />
            {live.connected ? "live" : "reconnecting…"}
          </div>
          <div className="status-actions">
            {saveError && (
              <span className="layout-error" role="alert">
                {saveError}
              </span>
            )}
            {editing ? (
              <>
                <button type="button" className="status-btn" onClick={reset}>
                  Reset layout
                </button>
                <button type="button" className="status-btn primary" onClick={done}>
                  Done
                </button>
              </>
            ) : (
              <button type="button" className="status-btn" onClick={() => setEditing(true)}>
                Edit layout
              </button>
            )}
            <button type="button" className="status-btn" onClick={reloadPage}>
              <span aria-hidden="true">↻ </span>Refresh
            </button>
          </div>
        </footer>
      </div>

      {tideOpen && live.tide && (
        <TideModal tide={live.tide} config={live.config?.tide} onClose={() => setTideOpen(false)} />
      )}
      {weatherOpen && live.weather && (
        <WeatherModal
          weather={live.weather}
          config={live.config?.weather}
          onClose={() => setWeatherOpen(false)}
        />
      )}
      {hotTubOpen && live.hottub && (
        <HotTubModal hottub={live.hottub} onClose={() => setHotTubOpen(false)} />
      )}
      {selectedEvent && (
        <EventModal event={selectedEvent} onClose={() => setSelectedEvent(null)} />
      )}
      {selectedDay && (
        <DayModal
          dayISO={selectedDay}
          events={live.events}
          onClose={() => setSelectedDay(null)}
          onEventClick={(ev) => {
            setSelectedDay(null);
            setSelectedEvent(ev);
          }}
        />
      )}
    </DisplayContext.Provider>
  );
}

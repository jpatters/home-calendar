import { useEffect, useRef, useState, type ReactNode } from "react";
import { Link } from "react-router-dom";
import type { Config } from "../types";
import type { LiveData } from "../useLiveData";
import { putConfig, refreshBaseball, refreshCalendars, refreshHockey, refreshHotTub, refreshPool, refreshSnowDay, refreshTide, refreshWeather } from "../api";
import CalendarsPanel from "./CalendarsPanel";
import WeatherPanel from "./WeatherPanel";
import SnowDayPanel from "./SnowDayPanel";
import TidePanel from "./TidePanel";
import BaseballPanel from "./BaseballPanel";
import HockeyPanel from "./HockeyPanel";
import PoolPanel from "./PoolPanel";
import HotTubPanel from "./HotTubPanel";
import DisplayPanel from "./DisplayPanel";
import AssistantPanel from "./AssistantPanel";

interface Props {
  live: LiveData;
}

type SectionId = "calendars" | "weather" | "tide" | "snowday" | "baseball" | "hockey" | "pool" | "hottub" | "assistant" | "display";

interface Section {
  id: SectionId;
  label: string;
  // Refresh is offered from the saved config, matching when the server's
  // refresh endpoint will accept the request.
  refresh?: {
    noun: string;
    run: () => Promise<void>;
    available: (c: Config) => boolean;
  };
}

const SECTIONS: Section[] = [
  { id: "calendars", label: "Calendars", refresh: { noun: "calendars", run: refreshCalendars, available: (c) => c.display.calendarEnabled } },
  { id: "weather", label: "Weather", refresh: { noun: "weather", run: refreshWeather, available: (c) => c.weather.enabled } },
  { id: "tide", label: "Tide", refresh: { noun: "tide", run: refreshTide, available: (c) => c.tide.enabled } },
  { id: "snowday", label: "Snow Day", refresh: { noun: "snow day", run: refreshSnowDay, available: (c) => c.snowDay.enabled } },
  { id: "baseball", label: "Baseball", refresh: { noun: "baseball", run: refreshBaseball, available: (c) => c.baseball.enabled && c.baseball.teamId !== 0 } },
  { id: "hockey", label: "Hockey", refresh: { noun: "hockey", run: refreshHockey, available: (c) => c.hockey.enabled && !!c.hockey.teamAbbr } },
  { id: "pool", label: "Pool", refresh: { noun: "pool", run: refreshPool, available: (c) => c.pool.enabled && !!c.pool.deviceUrl } },
  { id: "hottub", label: "Hot Tub", refresh: { noun: "hot tub", run: refreshHotTub, available: (c) => c.hotTub.enabled && !!c.hotTub.host } },
  { id: "assistant", label: "Assistant" },
  { id: "display", label: "Display" },
];

export default function Admin({ live }: Props) {
  const [draft, setDraft] = useState<Config | null>(live.config);
  const [sectionId, setSectionId] = useState<SectionId>("calendars");
  const [menuOpen, setMenuOpen] = useState(false);
  const menuButtonRef = useRef<HTMLButtonElement>(null);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [savedAt, setSavedAt] = useState<number | null>(null);

  useEffect(() => {
    if (live.config && !draft) setDraft(live.config);
  }, [live.config, draft]);

  useEffect(() => {
    if (!menuOpen) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape") return;
      setMenuOpen(false);
      menuButtonRef.current?.focus();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [menuOpen]);

  if (!draft) {
    return <div className="loading">Loading admin…</div>;
  }

  const save = async () => {
    setSaving(true);
    setError(null);
    try {
      const saved = await putConfig(draft);
      setDraft(saved);
      setSavedAt(Date.now());
    } catch (err) {
      setError(String(err));
    } finally {
      setSaving(false);
    }
  };

  const dirty = JSON.stringify(draft) !== JSON.stringify(live.config);
  const section = SECTIONS.find((s) => s.id === sectionId)!;
  const refresh = section.refresh;

  const actions =
    refresh && live.config && refresh.available(live.config) ? (
      <button
        type="button"
        className="refresh-btn"
        aria-label={`Refresh ${refresh.noun} now`}
        onClick={() => void refresh.run()}
      >
        <span aria-hidden="true">↻ </span>Refresh
      </button>
    ) : null;

  const choose = (id: SectionId) => {
    setSectionId(id);
    setMenuOpen(false);
  };

  return (
    <div className={`admin${menuOpen ? " menu-open" : ""}`}>
      <header className="admin-topbar">
        <button
          type="button"
          ref={menuButtonRef}
          className="admin-menu-btn"
          aria-label="Menu"
          aria-expanded={menuOpen}
          aria-controls="admin-sidebar"
          onClick={() => setMenuOpen((open) => !open)}
        >
          <span aria-hidden="true">☰</span>
        </button>
        <span className="admin-topbar-title">{section.label}</span>
      </header>

      <aside id="admin-sidebar" className="admin-sidebar">
        <h1 className="admin-title">Home Calendar · Admin</h1>
        <nav aria-label="Settings sections">
          <ul>
            {SECTIONS.map((s) => (
              <li key={s.id}>
                <button
                  type="button"
                  aria-current={s.id === sectionId ? "page" : undefined}
                  onClick={() => choose(s.id)}
                >
                  {s.label}
                </button>
              </li>
            ))}
          </ul>
        </nav>
        <Link className="admin-back" to="/">
          ← Back to display
        </Link>
      </aside>
      {menuOpen && <div className="admin-backdrop" onClick={() => setMenuOpen(false)} />}

      <main className="admin-main">
        <div className="admin-body">{renderSection(sectionId, draft, setDraft, actions)}</div>
        <footer className="admin-savebar">
          {error && <span className="error">{error}</span>}
          {savedAt && !dirty && <span className="saved">Saved</span>}
          <button className="primary" disabled={!dirty || saving} onClick={() => void save()}>
            {saving ? "Saving…" : "Save"}
          </button>
        </footer>
      </main>
    </div>
  );
}

function renderSection(
  id: SectionId,
  draft: Config,
  setDraft: (c: Config) => void,
  actions: ReactNode,
) {
  switch (id) {
    case "calendars":
      return (
        <CalendarsPanel
          actions={actions}
          value={draft.calendars}
          onChange={(calendars) => setDraft({ ...draft, calendars })}
        />
      );
    case "weather":
      return (
        <WeatherPanel
          actions={actions}
          value={draft.weather}
          onChange={(weather) => setDraft({ ...draft, weather })}
        />
      );
    case "tide":
      return (
        <TidePanel actions={actions} value={draft.tide} onChange={(tide) => setDraft({ ...draft, tide })} />
      );
    case "snowday":
      return (
        <SnowDayPanel
          actions={actions}
          value={draft.snowDay ?? { url: "" }}
          onChange={(snowDay) => setDraft({ ...draft, snowDay })}
        />
      );
    case "baseball":
      return (
        <BaseballPanel
          actions={actions}
          value={draft.baseball ?? { enabled: true, teamId: 0, teamName: "", teamAbbr: "" }}
          onChange={(baseball) => setDraft({ ...draft, baseball })}
        />
      );
    case "hockey":
      return (
        <HockeyPanel
          actions={actions}
          value={draft.hockey ?? { enabled: true, teamAbbr: "", teamName: "" }}
          onChange={(hockey) => setDraft({ ...draft, hockey })}
        />
      );
    case "pool":
      return (
        <PoolPanel
          actions={actions}
          value={draft.pool ?? { enabled: true, deviceUrl: "" }}
          onChange={(pool) => setDraft({ ...draft, pool })}
        />
      );
    case "hottub":
      return (
        <HotTubPanel
          actions={actions}
          value={draft.hotTub ?? { enabled: false, host: "" }}
          onChange={(hotTub) => setDraft({ ...draft, hotTub })}
        />
      );
    case "assistant":
      return <AssistantPanel value={draft.agent} onChange={(agent) => setDraft({ ...draft, agent })} />;
    case "display":
      return (
        <DisplayPanel
          actions={actions}
          value={draft.display}
          onChange={(display) => setDraft({ ...draft, display })}
          autoAvailable={
            draft.weather.enabled &&
            Boolean(draft.weather.location) &&
            (draft.weather.latitude !== 0 || draft.weather.longitude !== 0)
          }
        />
      );
  }
}

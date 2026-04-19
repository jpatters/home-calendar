import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import type { Config } from "../types";
import type { LiveData } from "../useLiveData";
import { putConfig, refreshCalendars, refreshWeather } from "../api";
import CalendarsPanel from "./CalendarsPanel";
import WeatherPanel from "./WeatherPanel";
import DisplayPanel from "./DisplayPanel";

interface Props {
  live: LiveData;
}

type Tab = "calendars" | "weather" | "display";

export default function Admin({ live }: Props) {
  const [draft, setDraft] = useState<Config | null>(live.config);
  const [tab, setTab] = useState<Tab>("calendars");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [savedAt, setSavedAt] = useState<number | null>(null);

  useEffect(() => {
    if (live.config && !draft) setDraft(live.config);
  }, [live.config, draft]);

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

  return (
    <div className="admin">
      <header className="admin-header">
        <h1>Home Calendar · Admin</h1>
        <nav>
          <Link to="/">← back to display</Link>
        </nav>
      </header>

      <div className="admin-tabs">
        <button className={tab === "calendars" ? "active" : ""} onClick={() => setTab("calendars")}>Calendars</button>
        <button className={tab === "weather" ? "active" : ""} onClick={() => setTab("weather")}>Weather</button>
        <button className={tab === "display" ? "active" : ""} onClick={() => setTab("display")}>Display</button>
      </div>

      <div className="admin-body">
        {tab === "calendars" && (
          <CalendarsPanel
            value={draft.calendars}
            onChange={(calendars) => setDraft({ ...draft, calendars })}
          />
        )}
        {tab === "weather" && (
          <WeatherPanel
            value={draft.weather}
            onChange={(weather) => setDraft({ ...draft, weather })}
          />
        )}
        {tab === "display" && (
          <DisplayPanel
            value={draft.display}
            onChange={(display) => setDraft({ ...draft, display })}
          />
        )}
      </div>

      <footer className="admin-footer">
        <div className="actions">
          <button onClick={() => void refreshCalendars()}>Refresh calendars now</button>
          <button onClick={() => void refreshWeather()}>Refresh weather now</button>
        </div>
        <div className="save">
          {error && <span className="error">{error}</span>}
          {savedAt && !dirty && <span className="saved">Saved</span>}
          <button className="primary" disabled={!dirty || saving} onClick={() => void save()}>
            {saving ? "Saving…" : "Save"}
          </button>
        </div>
      </footer>
    </div>
  );
}

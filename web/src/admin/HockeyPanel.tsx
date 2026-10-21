import { useEffect, useRef, useState } from "react";
import type { Hockey, HockeyTeam } from "../types";
import { hockeyTeamSearch } from "../api";

interface Props {
  value: Hockey;
  onChange: (h: Hockey) => void;
}

export default function HockeyPanel({ value, onChange }: Props) {
  const [query, setQuery] = useState(value.teamName);
  const [results, setResults] = useState<HockeyTeam[] | null>(null);
  const [searching, setSearching] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const abortRef = useRef<AbortController | null>(null);

  useEffect(() => {
    setQuery(value.teamName);
  }, [value.teamName]);

  useEffect(() => {
    const trimmed = query.trim();
    if (trimmed === "" || trimmed === value.teamName) {
      abortRef.current?.abort();
      setResults(null);
      setSearching(false);
      return;
    }
    const handle = setTimeout(() => {
      abortRef.current?.abort();
      const ctrl = new AbortController();
      abortRef.current = ctrl;
      setSearching(true);
      setError(null);
      hockeyTeamSearch(trimmed, ctrl.signal)
        .then((list) => {
          if (!ctrl.signal.aborted) {
            setResults(list);
            setSearching(false);
          }
        })
        .catch((err) => {
          if (ctrl.signal.aborted || err?.name === "AbortError") return;
          console.error("hockey team search failed", err);
          setError("Could not search. Try again.");
          setSearching(false);
        });
    }, 300);
    return () => clearTimeout(handle);
  }, [query, value.teamName]);

  const pick = (t: HockeyTeam) => {
    onChange({
      ...value,
      teamAbbr: t.abbrev,
      teamName: t.name,
    });
    setResults(null);
  };

  return (
    <div className={`panel${value.enabled ? "" : " panel-disabled"}`}>
      <h2>Hockey (NHL)</h2>
      <div className="toggle-row">
        <label className="toggle">
          <input
            type="checkbox"
            checked={value.enabled}
            onChange={(e) => onChange({ ...value, enabled: e.target.checked })}
          />
          Show hockey widget
        </label>
      </div>
      <p className="hint">
        Search for an NHL team — no API key required. The widget shows the
        latest completed game's score, the running score and clock while a
        game is live, and the date, time, and opponent of the next game.
      </p>
      <div className="form-grid">
        <label>
          <span>Team</span>
          <input
            type="text"
            autoComplete="off"
            placeholder="Start typing a team name (e.g. Maple Leafs, Oilers)"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        </label>
      </div>

      {searching && <div className="hint">Searching…</div>}
      {error && <div className="error">{error}</div>}
      {results && results.length === 0 && !searching && (
        <div className="hint">No matches found.</div>
      )}
      {results && results.length > 0 && (
        <ul className="geo-results">
          {results.map((t) => (
            <li key={t.abbrev}>
              <button type="button" onClick={() => pick(t)}>
                {t.name} ({t.abbrev})
              </button>
            </li>
          ))}
        </ul>
      )}

      <div className="hint">
        Saved team:{" "}
        <strong>
          {value.teamName ? `${value.teamName} (${value.teamAbbr})` : "(none)"}
        </strong>
      </div>
    </div>
  );
}

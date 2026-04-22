import { useEffect, useRef, useState } from "react";
import type { Baseball, BaseballTeam } from "../types";
import { baseballTeamSearch } from "../api";

interface Props {
  value: Baseball;
  onChange: (b: Baseball) => void;
}

function formatTeamLabel(t: BaseballTeam): string {
  return t.name;
}

export default function BaseballPanel({ value, onChange }: Props) {
  const [query, setQuery] = useState(value.teamName);
  const [results, setResults] = useState<BaseballTeam[] | null>(null);
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
      baseballTeamSearch(trimmed, ctrl.signal)
        .then((list) => {
          if (!ctrl.signal.aborted) {
            setResults(list);
            setSearching(false);
          }
        })
        .catch((err) => {
          if (ctrl.signal.aborted || err?.name === "AbortError") return;
          console.error("baseball team search failed", err);
          setError("Could not search. Try again.");
          setSearching(false);
        });
    }, 300);
    return () => clearTimeout(handle);
  }, [query, value.teamName]);

  const pick = (t: BaseballTeam) => {
    onChange({
      ...value,
      teamId: t.id,
      teamName: t.name,
      teamAbbr: t.abbreviation,
    });
    setResults(null);
  };

  return (
    <div className={`panel${value.enabled ? "" : " panel-disabled"}`}>
      <h2>Baseball (MLB Stats API)</h2>
      <div className="toggle-row">
        <label className="toggle">
          <input
            type="checkbox"
            checked={value.enabled}
            onChange={(e) => onChange({ ...value, enabled: e.target.checked })}
          />
          Show baseball widget
        </label>
      </div>
      <p className="hint">
        Search for an MLB team — no API key required. The widget shows the
        latest completed game's score and the date, time, and opponent of the
        next upcoming game.
      </p>
      <div className="form-grid">
        <label>
          <span>Team</span>
          <input
            type="text"
            autoComplete="off"
            placeholder="Start typing a team name (e.g. Yankees, Dodgers)"
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
            <li key={t.id}>
              <button type="button" onClick={() => pick(t)}>
                {formatTeamLabel(t)} ({t.abbreviation})
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

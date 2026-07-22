import { useEffect, useRef, useState } from "react";
import type { Tide, TideStation } from "../types";
import { tideStationSearch } from "../api";

interface Props {
  value: Tide;
  onChange: (t: Tide) => void;
}

export default function TidePanel({ value, onChange }: Props) {
  const [query, setQuery] = useState(value.location);
  const [results, setResults] = useState<TideStation[] | null>(null);
  const [searching, setSearching] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const abortRef = useRef<AbortController | null>(null);

  useEffect(() => {
    setQuery(value.location);
  }, [value.location]);

  useEffect(() => {
    const trimmed = query.trim();
    if (trimmed === "" || trimmed === value.location) {
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
      tideStationSearch(trimmed, ctrl.signal)
        .then((list) => {
          if (!ctrl.signal.aborted) {
            setResults(list);
            setSearching(false);
          }
        })
        .catch((err) => {
          if (ctrl.signal.aborted || err?.name === "AbortError") return;
          console.error("tide station search failed", err);
          setError("Could not search. Try again.");
          setSearching(false);
        });
    }, 300);
    return () => clearTimeout(handle);
  }, [query, value.location]);

  const pick = (s: TideStation) => {
    onChange({
      ...value,
      location: s.name,
      stationCode: s.code,
    });
    setResults(null);
  };

  return (
    <div className={`panel${value.enabled ? "" : " panel-disabled"}`}>
      <h2>Tides (CHS)</h2>
      <div className="toggle-row">
        <label className="toggle">
          <input
            type="checkbox"
            checked={value.enabled}
            onChange={(e) => onChange({ ...value, enabled: e.target.checked })}
          />
          Show tide widget
        </label>
      </div>
      <p className="hint">
        Search the Canadian Hydrographic Service tide stations — no API key
        required. Heights are metres above chart datum, matching published
        Canadian tide tables. Canadian stations only.
      </p>
      <div className="form-grid">
        <label>
          <span>Station</span>
          <input
            type="text"
            autoComplete="off"
            placeholder="Start typing a station or place name…"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        </label>
        <label>
          <span>Units</span>
          <select
            value={value.units}
            onChange={(e) =>
              onChange({ ...value, units: e.target.value as Tide["units"] })
            }
          >
            <option value="metric">Metric (metres)</option>
            <option value="imperial">Imperial (feet)</option>
          </select>
        </label>
      </div>

      {searching && <div className="hint">Searching…</div>}
      {error && <div className="error">{error}</div>}
      {results && results.length === 0 && !searching && (
        <div className="hint">No matches found.</div>
      )}
      {results && results.length > 0 && (
        <ul className="geo-results">
          {results.map((s) => (
            <li key={s.code}>
              <button type="button" onClick={() => pick(s)}>
                {s.name} ({s.code})
              </button>
            </li>
          ))}
        </ul>
      )}

      <div className="hint">
        Saved station: <strong>{value.location || "(none)"}</strong>{" "}
        {value.stationCode ? `(${value.stationCode})` : null}
      </div>
    </div>
  );
}

import { useEffect, useRef, useState } from "react";
import type { GeoResult, Tide } from "../types";
import { geocode } from "../api";

interface Props {
  value: Tide;
  onChange: (t: Tide) => void;
}

function formatLabel(r: GeoResult): string {
  return [r.name, r.admin1, r.country].filter(Boolean).join(", ");
}

export default function TidePanel({ value, onChange }: Props) {
  const [query, setQuery] = useState(value.location);
  const [results, setResults] = useState<GeoResult[] | null>(null);
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
      geocode(trimmed, ctrl.signal)
        .then((list) => {
          if (!ctrl.signal.aborted) {
            setResults(list);
            setSearching(false);
          }
        })
        .catch((err) => {
          if (ctrl.signal.aborted || err?.name === "AbortError") return;
          console.error("geocode search failed", err);
          setError("Could not search. Try again.");
          setSearching(false);
        });
    }, 300);
    return () => clearTimeout(handle);
  }, [query, value.location]);

  const pick = (r: GeoResult) => {
    onChange({
      ...value,
      location: formatLabel(r),
      latitude: r.latitude,
      longitude: r.longitude,
      timezone: r.timezone ?? value.timezone,
    });
    setResults(null);
  };

  return (
    <div className="panel">
      <h2>Tides (Open-Meteo Marine)</h2>
      <p className="hint">
        Search for a coastal location — no API key required. Note: this data
        is modelled at 8 km resolution and is not suitable for navigation.
      </p>
      <div className="form-grid">
        <label>
          <span>Location</span>
          <input
            type="text"
            autoComplete="off"
            placeholder="Start typing a coastal city…"
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
          {results.map((r, i) => (
            <li key={`${r.latitude},${r.longitude},${i}`}>
              <button type="button" onClick={() => pick(r)}>
                {formatLabel(r)}
              </button>
            </li>
          ))}
        </ul>
      )}

      <div className="hint">
        Saved location: <strong>{value.location || "(none)"}</strong>{" "}
        {value.latitude !== 0 || value.longitude !== 0
          ? `(${value.latitude.toFixed(2)}, ${value.longitude.toFixed(2)})`
          : null}
      </div>
    </div>
  );
}

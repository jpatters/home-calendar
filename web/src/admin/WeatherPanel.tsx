import { useEffect, useRef, useState } from "react";
import type { GeoResult, Weather } from "../types";
import { geocode } from "../api";

interface Props {
  value: Weather;
  onChange: (w: Weather) => void;
}

function formatLabel(r: GeoResult): string {
  return [r.name, r.admin1, r.country].filter(Boolean).join(", ");
}

export default function WeatherPanel({ value, onChange }: Props) {
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
    <div className={`panel${value.enabled ? "" : " panel-disabled"}`}>
      <h2>Weather (Open-Meteo)</h2>
      <div className="toggle-row">
        <label className="toggle">
          <input
            type="checkbox"
            checked={value.enabled}
            onChange={(e) => onChange({ ...value, enabled: e.target.checked })}
          />
          Show weather widget
        </label>
      </div>
      <p className="hint">Search for your city, town, or region — no API key required.</p>
      <div className="form-grid">
        <label>
          <span>Location</span>
          <input
            type="text"
            autoComplete="off"
            placeholder="Start typing a city name…"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        </label>
        <label>
          <span>Units</span>
          <select
            value={value.units}
            onChange={(e) => onChange({ ...value, units: e.target.value as Weather["units"] })}
          >
            <option value="metric">Metric (°C, km/h)</option>
            <option value="imperial">Imperial (°F, mph)</option>
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

      <div className="form-grid">
        <label style={{ gridColumn: "1 / -1" }}>
          <span>Ecowitt URL</span>
          <input
            type="url"
            autoComplete="off"
            placeholder="http://192.168.1.10/get_livedata_info"
            value={value.ecowittUrl}
            onChange={(e) => onChange({ ...value, ecowittUrl: e.target.value })}
          />
        </label>
      </div>
      <p className="hint">
        Optional. If set, the widget overlays real-time readings from a local Ecowitt
        gateway and refreshes every 60 seconds. Tap the widget to see the full live
        station detail. Leave blank to use Open-Meteo only.
      </p>
    </div>
  );
}

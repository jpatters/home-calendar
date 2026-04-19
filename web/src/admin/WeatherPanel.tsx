import type { Weather } from "../types";

interface Props {
  value: Weather;
  onChange: (w: Weather) => void;
}

export default function WeatherPanel({ value, onChange }: Props) {
  return (
    <div className="panel">
      <h2>Weather (Open-Meteo)</h2>
      <p className="hint">No API key required. Use decimal coordinates (e.g. Toronto: 43.65, -79.38).</p>
      <div className="form-grid">
        <label>
          <span>Latitude</span>
          <input
            type="number"
            step="0.0001"
            value={value.latitude}
            onChange={(e) => onChange({ ...value, latitude: Number(e.target.value) })}
          />
        </label>
        <label>
          <span>Longitude</span>
          <input
            type="number"
            step="0.0001"
            value={value.longitude}
            onChange={(e) => onChange({ ...value, longitude: Number(e.target.value) })}
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
        <label>
          <span>Timezone</span>
          <input
            type="text"
            placeholder="auto"
            value={value.timezone}
            onChange={(e) => onChange({ ...value, timezone: e.target.value })}
          />
        </label>
      </div>
    </div>
  );
}

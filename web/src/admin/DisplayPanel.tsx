import type { Display } from "../types";
import { MODE_LABELS, MODES, PALETTE_LABELS, PALETTES } from "../theme";

interface Props {
  value: Display;
  onChange: (d: Display) => void;
  autoAvailable: boolean;
}

export default function DisplayPanel({ value, onChange, autoAvailable }: Props) {
  return (
    <div className="panel">
      <h2>Display</h2>
      <div className="toggle-row">
        <label className="toggle">
          <input
            type="checkbox"
            checked={value.calendarEnabled}
            onChange={(e) => onChange({ ...value, calendarEnabled: e.target.checked })}
          />
          Show calendar
        </label>
        <label className="toggle">
          <input
            type="checkbox"
            checked={value.clockEnabled}
            onChange={(e) => onChange({ ...value, clockEnabled: e.target.checked })}
          />
          Show clock
        </label>
      </div>
      <div className="form-grid">
        <label>
          <span>Default view</span>
          <select
            value={value.defaultView}
            onChange={(e) => onChange({ ...value, defaultView: e.target.value as Display["defaultView"] })}
          >
            <option value="day">Day</option>
            <option value="week">Week</option>
            <option value="month">Month</option>
          </select>
        </label>
        <label>
          <span>Theme</span>
          <select
            value={value.theme}
            onChange={(e) => onChange({ ...value, theme: e.target.value as Display["theme"] })}
          >
            {PALETTES.map((p) => (
              <option key={p} value={p}>{PALETTE_LABELS[p]}</option>
            ))}
          </select>
        </label>
        <label>
          <span>Mode{!autoAvailable && " (set weather location for auto)"}</span>
          <select
            value={value.mode}
            onChange={(e) => onChange({ ...value, mode: e.target.value as Display["mode"] })}
          >
            {MODES.map((m) => (
              <option key={m} value={m} disabled={m === "auto" && !autoAvailable}>
                {MODE_LABELS[m]}
              </option>
            ))}
          </select>
        </label>
        <label>
          <span>Calendar refresh (seconds)</span>
          <input
            type="number"
            min={30}
            value={value.calendarRefreshSeconds}
            onChange={(e) => onChange({ ...value, calendarRefreshSeconds: Number(e.target.value) })}
          />
        </label>
        <label>
          <span>Weather refresh (seconds)</span>
          <input
            type="number"
            min={60}
            value={value.weatherRefreshSeconds}
            onChange={(e) => onChange({ ...value, weatherRefreshSeconds: Number(e.target.value) })}
          />
        </label>
      </div>
    </div>
  );
}

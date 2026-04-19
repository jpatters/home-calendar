import type { Display } from "../types";

interface Props {
  value: Display;
  onChange: (d: Display) => void;
}

export default function DisplayPanel({ value, onChange }: Props) {
  return (
    <div className="panel">
      <h2>Display</h2>
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
            <option value="light">Light</option>
            <option value="dark">Dark</option>
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

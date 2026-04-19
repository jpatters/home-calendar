import type { SnowDay } from "../types";

interface Props {
  value: SnowDay;
  onChange: (s: SnowDay) => void;
}

export default function SnowDayPanel({ value, onChange }: Props) {
  return (
    <div className="panel">
      <h2>Snow Day Predictor</h2>
      <p className="hint">
        Paste a location page URL from{" "}
        <a href="https://www.snowdaypredictor.com" target="_blank" rel="noreferrer">
          snowdaypredictor.com
        </a>{" "}
        (e.g. <code>https://www.snowdaypredictor.com/prediction/canoe-cove-pe</code>). Leave blank
        to disable the widget.
      </p>
      <div className="form-grid">
        <label style={{ gridColumn: "1 / -1" }}>
          <span>Prediction URL</span>
          <input
            type="url"
            placeholder="https://www.snowdaypredictor.com/prediction/..."
            value={value.url}
            onChange={(e) => onChange({ ...value, url: e.target.value })}
          />
        </label>
      </div>
    </div>
  );
}

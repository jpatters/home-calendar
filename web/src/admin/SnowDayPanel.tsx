import type { ReactNode } from "react";
import type { SnowDay } from "../types";

interface Props {
  value: SnowDay;
  onChange: (s: SnowDay) => void;
  actions?: ReactNode;
}

export default function SnowDayPanel({ actions, value, onChange }: Props) {
  return (
    <div className={`panel${value.enabled ? "" : " panel-disabled"}`}>
      <div className="panel-header">
        <h2>Snow Day Predictor</h2>
        {actions}
      </div>
      <div className="toggle-row">
        <label className="toggle">
          <input
            type="checkbox"
            checked={value.enabled}
            onChange={(e) => onChange({ ...value, enabled: e.target.checked })}
          />
          Show snow day widget
        </label>
      </div>
      <p className="hint">
        Paste a location page URL from{" "}
        <a href="https://www.snowdaypredictor.com" target="_blank" rel="noreferrer">
          snowdaypredictor.com
        </a>{" "}
        (e.g. <code>https://www.snowdaypredictor.com/prediction/canoe-cove-pe</code>).
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

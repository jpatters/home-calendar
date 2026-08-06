import type { Pool } from "../types";

interface Props {
  value: Pool;
  onChange: (p: Pool) => void;
}

export default function PoolPanel({ value, onChange }: Props) {
  return (
    <div className={`panel${value.enabled ? "" : " panel-disabled"}`}>
      <h2>Pool (ESPHome)</h2>
      <div className="toggle-row">
        <label className="toggle">
          <input
            type="checkbox"
            checked={value.enabled}
            onChange={(e) => onChange({ ...value, enabled: e.target.checked })}
          />
          Show pool widget
        </label>
      </div>
      <p className="hint">
        Reads the pool heater's climate entity from your ESPHome device's built-in
        web server. Point this at the device's address on your network — a fixed
        IP is more reliable than an mDNS <code>.local</code> name.
      </p>
      <div className="form-grid">
        <label>
          <span>Device URL</span>
          <input
            type="text"
            autoComplete="off"
            placeholder="http://pool-thermostat.local"
            value={value.deviceUrl}
            onChange={(e) => onChange({ ...value, deviceUrl: e.target.value })}
          />
        </label>
      </div>
    </div>
  );
}

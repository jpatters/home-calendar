import type { ReactNode } from "react";
import type { HotTub } from "../types";

interface Props {
  value: HotTub;
  onChange: (h: HotTub) => void;
  actions?: ReactNode;
}

export default function HotTubPanel({ actions, value, onChange }: Props) {
  return (
    <div className={`panel${value.enabled ? "" : " panel-disabled"}`}>
      <div className="panel-header">
        <h2>Hot tub (in.touch2)</h2>
        {actions}
      </div>
      <div className="toggle-row">
        <label className="toggle">
          <input
            type="checkbox"
            checked={value.enabled}
            onChange={(e) => onChange({ ...value, enabled: e.target.checked })}
          />
          Show hot tub widget
        </label>
      </div>
      <p className="hint">
        Reads water temperature, setpoint, and heater state straight from a Gecko
        in.touch2 WiFi module on your network (UDP port 10022). Enter the module's
        IP address; add <code>:port</code> only if you have changed it.
      </p>
      <div className="form-grid">
        <label>
          <span>Module address</span>
          <input
            type="text"
            autoComplete="off"
            placeholder="192.168.1.50"
            value={value.host}
            onChange={(e) => onChange({ ...value, host: e.target.value })}
          />
        </label>
      </div>
    </div>
  );
}

import { useEffect, useState } from "react";
import type { Agent, AgentEffort, AgentSnapshot } from "../types";
import { getAgentStatus } from "../api";

interface Props {
  value: Agent;
  onChange: (a: Agent) => void;
}

const EFFORTS: AgentEffort[] = ["none", "minimal", "low", "medium", "high", "xhigh", "max"];

// Secrets are configured only through the server's environment; this panel
// reports whether they are present and never shows or accepts their values.
export default function AssistantPanel({ value, onChange }: Props) {
  const [status, setStatus] = useState<AgentSnapshot | null>(null);

  useEffect(() => {
    let cancelled = false;
    getAgentStatus().then(
      (s) => {
        if (!cancelled) setStatus(s);
      },
      () => {},
    );
    return () => {
      cancelled = true;
    };
  }, []);

  return (
    <div className={`panel${value.enabled ? "" : " panel-disabled"}`}>
      <h2>Assistant (Ziggy)</h2>
      <div className="toggle-row">
        <label className="toggle">
          <input
            type="checkbox"
            checked={value.enabled}
            onChange={(e) => onChange({ ...value, enabled: e.target.checked })}
          />
          Enable Ziggy
        </label>
        <label className="toggle">
          <input
            type="checkbox"
            checked={value.textInputEnabled}
            onChange={(e) => onChange({ ...value, textInputEnabled: e.target.checked })}
          />
          Let people type to Ziggy on the display
        </label>
      </div>
      {status && (
        <div className="hint">
          <div>{`OpenAI key: ${status.ready ? "set" : "missing"} (OPENAI_API_KEY)`}</div>
          <div>
            {`Calendar changes: ${status.calendarWritable ? "available" : "unavailable"} (GOOGLE_SERVICE_ACCOUNT_JSON)`}
          </div>
        </div>
      )}
      <div className="form-grid">
        <label>
          <span>Model</span>
          <input
            type="text"
            autoComplete="off"
            value={value.model}
            onChange={(e) => onChange({ ...value, model: e.target.value })}
          />
        </label>
        <label>
          <span>Reasoning effort</span>
          <select
            value={value.effort}
            onChange={(e) => onChange({ ...value, effort: e.target.value as AgentEffort })}
          >
            {EFFORTS.map((effort) => (
              <option key={effort} value={effort}>{effort}</option>
            ))}
          </select>
        </label>
        <label>
          <span>Quiet hours start</span>
          <input
            type="time"
            value={value.quietStart}
            onChange={(e) => onChange({ ...value, quietStart: e.target.value })}
          />
        </label>
        <label>
          <span>Quiet hours end</span>
          <input
            type="time"
            value={value.quietEnd}
            onChange={(e) => onChange({ ...value, quietEnd: e.target.value })}
          />
        </label>
      </div>
    </div>
  );
}

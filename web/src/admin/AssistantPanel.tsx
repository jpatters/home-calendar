import { useEffect, useState } from "react";
import type { Agent, AgentAuth, AgentEffort } from "../types";
import { getAgentAuth } from "../api";
import AssistantConnection from "./AssistantConnection";

interface Props {
  value: Agent;
  onChange: (a: Agent) => void;
}

const EFFORTS: AgentEffort[] = ["none", "minimal", "low", "medium", "high", "xhigh", "max"];

export default function AssistantPanel({ value, onChange }: Props) {
  const [auth, setAuth] = useState<AgentAuth | null>(null);
  const [statusFailed, setStatusFailed] = useState(false);

  useEffect(() => {
    let cancelled = false;
    getAgentAuth().then(
      (a) => {
        if (!cancelled) setAuth(a);
      },
      () => {
        if (!cancelled) setStatusFailed(true);
      },
    );
    return () => {
      cancelled = true;
    };
  }, []);

  return (
    <div className={`panel${value.enabled ? "" : " panel-disabled"}`}>
      <div className="panel-header">
        <h2>Assistant (Ziggy)</h2>
      </div>
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
      {statusFailed && <div className="hint">Status unavailable</div>}
      {auth && <AssistantConnection auth={auth} onAuth={setAuth} />}
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

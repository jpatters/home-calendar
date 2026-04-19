import type { Tide, TideEvent, TideSnapshot } from "../types";
import { formatHeight, formatTime } from "./tideFormat";

interface Props {
  tide: TideSnapshot;
  config: Tide | undefined;
  onClose: () => void;
}

function dayKey(iso: string): string {
  const d = new Date(iso);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

function dayLabel(iso: string): string {
  return new Date(iso).toLocaleDateString([], {
    weekday: "long",
    month: "short",
    day: "numeric",
  });
}

function groupByDay(events: TideEvent[]): Array<{ key: string; sample: string; events: TideEvent[] }> {
  const out: Array<{ key: string; sample: string; events: TideEvent[] }> = [];
  const byKey = new Map<string, { key: string; sample: string; events: TideEvent[] }>();
  for (const ev of events) {
    const key = dayKey(ev.time);
    const group = byKey.get(key);
    if (group) {
      group.events.push(ev);
    } else {
      const fresh = { key, sample: ev.time, events: [ev] };
      byKey.set(key, fresh);
      out.push(fresh);
    }
  }
  return out;
}

export default function TideModal({ tide, config, onClose }: Props) {
  const units = config?.units ?? tide.units;
  const grouped = groupByDay(tide.events);
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div
        className="modal tide-modal"
        role="dialog"
        aria-label="Tide forecast"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="modal-header">
          <h2>Tides{config?.location ? ` · ${config.location}` : ""}</h2>
          <button
            type="button"
            className="close-btn"
            aria-label="Close"
            onClick={onClose}
          >
            ×
          </button>
        </div>
        <div className="modal-body tide-modal-body">
          {grouped.map((g) => (
            <section key={g.key} className="tide-day">
              <h3 className="tide-day-label">{dayLabel(g.sample)}</h3>
              <ul className="tide-day-events">
                {g.events.map((ev) => (
                  <li key={`${ev.time}-${ev.type}`} className="tide-event">
                    <span className="tide-icon" aria-hidden>
                      {ev.type === "high" ? "↑" : "↓"}
                    </span>
                    <span className="tide-type">
                      {ev.type === "high" ? "High" : "Low"}
                    </span>
                    <span className="tide-time">{formatTime(ev.time)}</span>
                    <span className="tide-height">
                      {formatHeight(ev.heightMeters, units)}
                    </span>
                  </li>
                ))}
              </ul>
            </section>
          ))}
        </div>
      </div>
    </div>
  );
}

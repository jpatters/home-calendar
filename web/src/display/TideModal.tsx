import { useState } from "react";
import type { Tide, TideEvent, TideSnapshot } from "../types";
import { formatHeight, formatTime } from "./tideFormat";
import { useSwipe } from "./useSwipe";

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
  const [index, setIndex] = useState(0);
  const units = config?.units ?? tide.units;
  const groups = groupByDay(tide.events);

  const last = Math.max(0, groups.length - 1);
  const go = (delta: number) => setIndex((i) => Math.min(last, Math.max(0, i + delta)));
  const swipe = useSwipe({ onNext: () => go(1), onPrev: () => go(-1) });

  const group = groups[index];

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div
        className="modal tide-modal"
        role="dialog"
        aria-label="Tide forecast"
        onClick={(e) => e.stopPropagation()}
        {...swipe}
      >
        <div className="modal-header tide-modal-header">
          <button
            type="button"
            className="modal-nav-btn"
            aria-label="Previous day"
            onClick={() => go(-1)}
            disabled={index === 0}
          >
            ‹
          </button>
          <h2 className="tide-modal-title">
            {group ? dayLabel(group.sample) : `Tides${config?.location ? ` · ${config.location}` : ""}`}
          </h2>
          <button
            type="button"
            className="modal-nav-btn"
            aria-label="Next day"
            onClick={() => go(1)}
            disabled={index >= last}
          >
            ›
          </button>
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
          {group ? (
            <ul className="tide-day-events">
              {group.events.map((ev) => (
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
          ) : (
            <div>No tide data</div>
          )}
        </div>
      </div>
    </div>
  );
}

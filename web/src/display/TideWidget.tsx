import type { Tide, TideEvent, TideSnapshot } from "../types";
import { formatHeight, formatTime } from "./tideFormat";

interface Props {
  tide: TideSnapshot | null;
  config: Tide | undefined;
  onOpen: () => void;
}

function EventRow({ event, units }: { event: TideEvent; units: string | undefined }) {
  const arrow = event.type === "high" ? "↑" : "↓";
  const label = event.type === "high" ? "High" : "Low";
  return (
    <div className="tide-event">
      <span className="tide-icon" aria-hidden>{arrow}</span>
      <span className="tide-type">{label}</span>
      <span className="tide-time">{formatTime(event.time)}</span>
      <span className="tide-height">{formatHeight(event.heightMeters, units)}</span>
    </div>
  );
}

export default function TideWidget({ tide, config, onOpen }: Props) {
  if (!tide || tide.events.length === 0) {
    return (
      <div className="widget tide-widget tide-widget-empty">
        <div className="tide-empty">Tide unavailable</div>
      </div>
    );
  }
  const units = config?.units ?? tide.units;
  const upcoming = tide.events.slice(0, 4);
  return (
    <button
      type="button"
      className="widget tide-widget"
      aria-label="Tide details"
      onClick={onOpen}
    >
      {config?.location && (
        <div className="tide-location">{config.location}</div>
      )}
      <div className="tide-current">
        Now: {formatHeight(tide.currentMeters, units)}
      </div>
      <div className="tide-events">
        {upcoming.map((ev) => (
          <EventRow key={`${ev.time}-${ev.type}`} event={ev} units={units} />
        ))}
      </div>
    </button>
  );
}

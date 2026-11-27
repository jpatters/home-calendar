import type { PoolSnapshot } from "../types";
import { formatTemp } from "./poolFormat";

interface Props {
  pool: PoolSnapshot | null;
  onOpen: () => void;
}

export default function PoolWidget({ pool, onOpen }: Props) {
  if (!pool) {
    return (
      <div className="widget pool-widget pool-widget-empty">
        <div className="pool-empty">Pool unavailable</div>
      </div>
    );
  }
  return (
    <button type="button" className="widget pool-widget" aria-label="Pool details" onClick={onOpen}>
      <div className="pool-header">
        <span className="pool-label">Pool</span>
        <span
          className={`pool-state ${pool.heating ? "pool-heating" : "pool-idle"}`}
        >
          {pool.heating ? "Heating" : "Idle"}
        </span>
      </div>
      <div className="pool-temp-value">{formatTemp(pool.temperatureC)}</div>
      {pool.targetC > 0 && (
        <div className="pool-target">Target {formatTemp(pool.targetC)}</div>
      )}
    </button>
  );
}

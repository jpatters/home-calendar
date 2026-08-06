import type { PoolSnapshot } from "../types";

interface Props {
  pool: PoolSnapshot | null;
}

function formatTemp(celsius: number): string {
  return `${celsius.toFixed(1)}°C`;
}

export default function PoolWidget({ pool }: Props) {
  if (!pool) {
    return (
      <div className="widget pool-widget pool-widget-empty">
        <div className="pool-empty">Pool unavailable</div>
      </div>
    );
  }
  return (
    <div className="widget pool-widget">
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
    </div>
  );
}

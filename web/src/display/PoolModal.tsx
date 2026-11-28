import type { PoolSnapshot } from "../types";
import { setPoolTarget } from "../api";
import { formatTemp, stepTarget } from "./poolFormat";
import { useSetpoint } from "./useSetpoint";

interface Props {
  pool: PoolSnapshot;
  onClose: () => void;
}

export default function PoolModal({ pool, onClose }: Props) {
  const { target, error, canRaise, canLower, adjust, flush } = useSetpoint({
    live: Math.round(pool.targetC * 10) / 10,
    min: pool.minTargetC,
    max: pool.maxTargetC,
    step: stepTarget,
    write: setPoolTarget,
  });

  const close = () => {
    flush();
    onClose();
  };

  return (
    <div className="modal-backdrop" onClick={close}>
      <div
        className="modal pool-modal"
        role="dialog"
        aria-label="Pool"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="modal-header">
          <h2>Pool</h2>
          <span className={`pool-state ${pool.heating ? "pool-heating" : "pool-idle"}`}>
            {pool.heating ? "Heating" : "Idle"}
          </span>
          <button type="button" className="close-btn" aria-label="Close" onClick={close}>
            ×
          </button>
        </div>
        <div className="modal-body pool-modal-body">
          <div className="pool-modal-reading">
            <div className="pool-modal-caption">Water</div>
            <div className="pool-modal-temp">{formatTemp(pool.temperatureC)}</div>
          </div>
          <div className="pool-modal-reading">
            <div className="pool-modal-caption">Target</div>
            <div className="pool-modal-control">
              <button
                type="button"
                className="pool-arrow-btn"
                aria-label="Lower target"
                onClick={() => adjust(-1)}
                disabled={!canLower}
              >
                ▼
              </button>
              <div className="pool-modal-temp pool-modal-target">{formatTemp(target)}</div>
              <button
                type="button"
                className="pool-arrow-btn"
                aria-label="Raise target"
                onClick={() => adjust(1)}
                disabled={!canRaise}
              >
                ▲
              </button>
            </div>
          </div>
          {error && <div className="pool-modal-error" role="alert">{error}</div>}
        </div>
      </div>
    </div>
  );
}

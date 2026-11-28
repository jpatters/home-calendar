import type { HotTubSnapshot } from "../types";
import { setHotTubTarget } from "../api";
import { formatTemp } from "./hotTubFormat";
import { useSetpoint } from "./useSetpoint";

interface Props {
  hottub: HotTubSnapshot;
  onClose: () => void;
}

export default function HotTubModal({ hottub, onClose }: Props) {
  const { target, error, canRaise, canLower, adjust, flush } = useSetpoint({
    live: Math.round(hottub.targetF),
    min: hottub.minTargetF,
    max: hottub.maxTargetF,
    step: (t, d) => t + d,
    write: setHotTubTarget,
  });

  const close = () => {
    flush();
    onClose();
  };

  return (
    <div className="modal-backdrop" onClick={close}>
      <div
        className="modal hottub-modal"
        role="dialog"
        aria-label="Hot tub"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="modal-header">
          <h2>Hot tub</h2>
          <span
            className={`hottub-state ${hottub.heating ? "hottub-heating" : "hottub-idle"}`}
          >
            {hottub.heating ? "Heating" : "Idle"}
          </span>
          <button type="button" className="close-btn" aria-label="Close" onClick={close}>
            ×
          </button>
        </div>
        <div className="modal-body hottub-modal-body">
          <div className="hottub-modal-reading">
            <div className="hottub-modal-caption">Water</div>
            <div className="hottub-modal-temp">{formatTemp(hottub.temperatureF)}</div>
          </div>
          <div className="hottub-modal-reading">
            <div className="hottub-modal-caption">Target</div>
            <div className="hottub-modal-control">
              <button
                type="button"
                className="hottub-arrow-btn"
                aria-label="Lower target"
                onClick={() => adjust(-1)}
                disabled={!canLower}
              >
                ▼
              </button>
              <div className="hottub-modal-temp hottub-modal-target">{formatTemp(target)}</div>
              <button
                type="button"
                className="hottub-arrow-btn"
                aria-label="Raise target"
                onClick={() => adjust(1)}
                disabled={!canRaise}
              >
                ▲
              </button>
            </div>
          </div>
          {error && <div className="hottub-modal-error" role="alert">{error}</div>}
        </div>
      </div>
    </div>
  );
}

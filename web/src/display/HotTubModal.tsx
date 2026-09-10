import { useEffect, useRef, useState } from "react";
import type { HotTubSnapshot } from "../types";
import { setHotTubTarget } from "../api";
import { formatTemp } from "./hotTubFormat";

interface Props {
  hottub: HotTubSnapshot;
  onClose: () => void;
}

// Taps within this window collapse into a single request carrying the final
// target, so a burst of taps is one UDP write to the tub instead of many.
const SEND_DELAY_MS = 600;

export default function HotTubModal({ hottub, onClose }: Props) {
  // The target the user has chosen here, shown in place of the live value
  // until the tub reports a change of its own.
  const [chosen, setChosen] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const inFlight = useRef(0);

  const liveTarget = Math.round(hottub.targetF);
  const liveRef = useRef(liveTarget);
  liveRef.current = liveTarget;
  const target = chosen ?? liveTarget;
  const canRaise = target < hottub.maxTargetF;
  const canLower = target > hottub.minTargetF;

  // A live change while a tap is waiting to be sent, or a write is still in
  // flight, may just be the echo of an earlier write; the newest tap wins.
  useEffect(() => {
    if (timer.current === null && inFlight.current === 0) {
      setChosen(null);
    }
  }, [liveTarget]);

  const cancelTimer = () => {
    if (timer.current !== null) {
      clearTimeout(timer.current);
      timer.current = null;
    }
  };

  const send = (value: number) => {
    cancelTimer();
    if (value === liveRef.current) {
      setChosen(null);
      return;
    }
    inFlight.current += 1;
    setHotTubTarget(value).then(
      () => {
        inFlight.current -= 1;
      },
      () => {
        inFlight.current -= 1;
        setChosen(null);
        setError("Couldn't update the target. Try again.");
      },
    );
  };

  const adjust = (delta: number) => {
    const next = Math.min(hottub.maxTargetF, Math.max(hottub.minTargetF, target + delta));
    setChosen(next);
    setError(null);
    cancelTimer();
    timer.current = setTimeout(() => send(next), SEND_DELAY_MS);
  };

  const close = () => {
    if (timer.current !== null && chosen !== null) {
      send(chosen);
    }
    onClose();
  };

  useEffect(() => cancelTimer, []);

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

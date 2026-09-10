import type { HotTubSnapshot } from "../types";
import { formatTemp } from "./hotTubFormat";

interface Props {
  hottub: HotTubSnapshot | null;
  onOpen: () => void;
}

export default function HotTubWidget({ hottub, onOpen }: Props) {
  if (!hottub) {
    return (
      <div className="widget hottub-widget hottub-widget-empty">
        <div className="hottub-empty">Hot tub unavailable</div>
      </div>
    );
  }
  return (
    <button
      type="button"
      className="widget hottub-widget"
      aria-label="Hot tub details"
      onClick={onOpen}
    >
      <div className="hottub-header">
        <span className="hottub-label">Hot tub</span>
        <span
          className={`hottub-state ${hottub.heating ? "hottub-heating" : "hottub-idle"}`}
        >
          {hottub.heating ? "Heating" : "Idle"}
        </span>
      </div>
      <div className="hottub-temp-value">{formatTemp(hottub.temperatureF)}</div>
      <div className="hottub-target">Target {formatTemp(hottub.targetF)}</div>
    </button>
  );
}

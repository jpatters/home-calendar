import type { HotTubSnapshot } from "../types";

interface Props {
  hottub: HotTubSnapshot | null;
}

function formatTemp(fahrenheit: number): string {
  return `${Math.round(fahrenheit)}°F`;
}

export default function HotTubWidget({ hottub }: Props) {
  if (!hottub) {
    return (
      <div className="widget hottub-widget hottub-widget-empty">
        <div className="hottub-empty">Hot tub unavailable</div>
      </div>
    );
  }
  return (
    <div className="widget hottub-widget">
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
    </div>
  );
}

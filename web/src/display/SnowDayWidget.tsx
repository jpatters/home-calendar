import type { SnowDay, SnowDaySnapshot } from "../types";

interface Props {
  snowday: SnowDaySnapshot | null;
  config: SnowDay | undefined;
}

function morningLabel(iso: string): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleDateString([], { weekday: "short", month: "short", day: "numeric" });
}

export default function SnowDayWidget({ snowday, config }: Props) {
  if (!config?.url) return null;

  if (!snowday) {
    return (
      <div className="widget snowday-widget">
        <div className="snowday-empty">Snow day prediction unavailable</div>
      </div>
    );
  }

  const morning = morningLabel(snowday.morningTime);
  const meta = [morning, snowday.location].filter(Boolean).join(" · ");

  return (
    <div className="widget snowday-widget">
      <div className="snowday-icon" aria-hidden>❄</div>
      <div className="snowday-percent">{snowday.probability}%</div>
      <div className="snowday-label">chance of snow day</div>
      {snowday.category && <div className="snowday-category">{snowday.category}</div>}
      {meta && <div className="snowday-meta">{meta}</div>}
    </div>
  );
}

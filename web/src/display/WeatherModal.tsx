import { useState } from "react";
import type { Weather, WeatherDaily, WeatherSnapshot } from "../types";
import { labelForCode, WeatherIcon } from "./weatherIcons";
import { precipUnit, speedUnit, tempUnit } from "./weatherFormat";
import { useSwipe } from "./useSwipe";

interface Props {
  weather: WeatherSnapshot;
  config: Weather | undefined;
  onClose: () => void;
}

function dayHeading(iso: string, index: number): string {
  if (index === 0) return "Today";
  if (index === 1) return "Tomorrow";
  const d = new Date(iso + "T00:00:00");
  return d.toLocaleDateString([], { weekday: "short", month: "short", day: "numeric" }).replace(",", " ·");
}

function formatTime(iso: string): string {
  if (!iso) return "—";
  // Open-Meteo returns "YYYY-MM-DDTHH:MM" without timezone. Parse as local.
  const d = new Date(iso);
  if (isNaN(d.getTime())) return "—";
  return d.toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
}

function formatPrecip(mm: number, units: string | undefined): string {
  if (mm <= 0) return "No precipitation";
  if (units === "imperial") {
    const inches = mm / 25.4;
    return `${inches.toFixed(2)} ${precipUnit(units)}`;
  }
  return `${mm.toFixed(1)} ${precipUnit(units)}`;
}

function DayPage({ day, units }: { day: WeatherDaily; units: string | undefined }) {
  return (
    <div className="weather-modal-day">
      <WeatherIcon code={day.weatherCode} className="weather-modal-icon" />
      <div className="weather-modal-label">{labelForCode(day.weatherCode)}</div>
      <div className="weather-modal-temps">
        <span className="weather-modal-max">{Math.round(day.maxC)} {tempUnit(units)}</span>
        <span className="weather-modal-sep"> / </span>
        <span className="weather-modal-min">{Math.round(day.minC)} {tempUnit(units)}</span>
      </div>
      <dl className="weather-modal-stats">
        <div>
          <dt>Wind</dt>
          <dd>{Math.round(day.windSpeedMax)} {speedUnit(units)}</dd>
        </div>
        <div>
          <dt>Precipitation</dt>
          <dd>{formatPrecip(day.precipMM, units)}</dd>
        </div>
        <div>
          <dt>Sunrise</dt>
          <dd>{formatTime(day.sunrise)}</dd>
        </div>
        <div>
          <dt>Sunset</dt>
          <dd>{formatTime(day.sunset)}</dd>
        </div>
      </dl>
    </div>
  );
}

export default function WeatherModal({ weather, config, onClose }: Props) {
  const [index, setIndex] = useState(0);
  const units = config?.units ?? weather.units;
  const days = weather.daily;

  const last = Math.max(0, days.length - 1);
  const go = (delta: number) => setIndex((i) => Math.min(last, Math.max(0, i + delta)));
  const swipe = useSwipe({ onNext: () => go(1), onPrev: () => go(-1) });

  const day = days[index];

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div
        className="modal weather-modal"
        role="dialog"
        aria-label="Weather forecast"
        onClick={(e) => e.stopPropagation()}
        {...swipe}
      >
        <div className="modal-header weather-modal-header">
          <button
            type="button"
            className="modal-nav-btn"
            aria-label="Previous day"
            onClick={() => go(-1)}
            disabled={index === 0}
          >
            ‹
          </button>
          <h2 className="weather-modal-title">
            {day ? dayHeading(day.date, index) : "Weather"}
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
        <div className="modal-body weather-modal-body">
          {day ? <DayPage day={day} units={units} /> : <div>No forecast available</div>}
        </div>
      </div>
    </div>
  );
}

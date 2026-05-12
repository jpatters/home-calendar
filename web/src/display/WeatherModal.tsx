import { useState } from "react";
import type { Weather, WeatherDaily, WeatherSnapshot, WeatherStation } from "../types";
import { labelForCode, WeatherIcon } from "./weatherIcons";
import { compassFromDegrees, precipUnit, speedUnit, tempUnit } from "./weatherFormat";
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

function formatStationUpdatedAt(iso: string): string {
  const d = new Date(iso);
  if (isNaN(d.getTime())) return "—";
  return d.toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
}

function StationPage({ station, units }: { station: WeatherStation; units: string | undefined }) {
  const imperial = units === "imperial";
  const rainPerHr = imperial ? "in/h" : "mm/h";
  const rainUnit = imperial ? "in" : "mm";
  const fmt = (n: number, digits = 1) => n.toFixed(digits);
  return (
    <div className="weather-modal-day">
      <div className="weather-modal-label">
        Local Ecowitt gateway · updated {formatStationUpdatedAt(station.updatedAt)}
      </div>
      <dl className="weather-modal-stats">
        {station.hasIndoor && (
          <>
            <div>
              <dt>Indoor temp</dt>
              <dd>{fmt(station.indoorTempC)} {tempUnit(units)}</dd>
            </div>
            <div>
              <dt>Indoor humidity</dt>
              <dd>{station.indoorHumidity} %</dd>
            </div>
            <div>
              <dt>Pressure</dt>
              <dd>{fmt(station.pressureHPa)} hPa</dd>
            </div>
          </>
        )}
        {station.hasOutdoor && (
          <>
            <div>
              <dt>Wind gust</dt>
              <dd>{fmt(station.windGust)} {speedUnit(units)}</dd>
            </div>
            <div>
              <dt>Wind direction</dt>
              <dd>{station.windDirection}° {compassFromDegrees(station.windDirection)}</dd>
            </div>
            <div>
              <dt>Solar</dt>
              <dd>{fmt(station.solarWM2)} W/m²</dd>
            </div>
            <div>
              <dt>Rain rate</dt>
              <dd>{fmt(station.rainRate, 2)} {rainPerHr}</dd>
            </div>
            <div>
              <dt>Rain event</dt>
              <dd>{fmt(station.rainEvent, 2)} {rainUnit}</dd>
            </div>
            <div>
              <dt>Rain today</dt>
              <dd>{fmt(station.rainDaily, 2)} {rainUnit}</dd>
            </div>
            <div>
              <dt>Rain week</dt>
              <dd>{fmt(station.rainWeekly, 2)} {rainUnit}</dd>
            </div>
            <div>
              <dt>Rain month</dt>
              <dd>{fmt(station.rainMonthly, 2)} {rainUnit}</dd>
            </div>
            <div>
              <dt>Rain year</dt>
              <dd>{fmt(station.rainYearly, 1)} {rainUnit}</dd>
            </div>
          </>
        )}
      </dl>
    </div>
  );
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
  const station = weather.station ?? null;
  const stationIndex = station ? days.length : -1;
  const totalPages = days.length + (station ? 1 : 0);

  const last = Math.max(0, totalPages - 1);
  const go = (delta: number) => setIndex((i) => Math.min(last, Math.max(0, i + delta)));
  const swipe = useSwipe({ onNext: () => go(1), onPrev: () => go(-1) });

  const isStationPage = station != null && index === stationIndex;
  const day = isStationPage ? undefined : days[index];
  const heading = isStationPage
    ? "Live Station"
    : day
      ? dayHeading(day.date, index)
      : "Weather";

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
          <h2 className="weather-modal-title">{heading}</h2>
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
          {isStationPage && station ? (
            <StationPage station={station} units={units} />
          ) : day ? (
            <DayPage day={day} units={units} />
          ) : (
            <div>No forecast available</div>
          )}
        </div>
      </div>
    </div>
  );
}

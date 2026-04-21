import type { Weather, WeatherSnapshot } from "../types";
import { labelForCode, WeatherIcon } from "./weatherIcons";
import { speedUnit, tempUnit } from "./weatherFormat";

interface Props {
  weather: WeatherSnapshot | null;
  config: Weather | undefined;
  onOpen: () => void;
}

function dayLabel(iso: string): string {
  const d = new Date(iso + "T00:00:00");
  return d.toLocaleDateString([], { weekday: "short" });
}

export default function WeatherWidget({ weather, config, onOpen }: Props) {
  if (!weather) {
    return (
      <div className="widget weather-widget">
        <div className="weather-empty">Weather unavailable</div>
      </div>
    );
  }
  const units = config?.units ?? weather.units;
  return (
    <button
      type="button"
      className="widget weather-widget"
      aria-label="Weather details"
      onClick={onOpen}
    >
      <div className="weather-current">
        {config?.location && (
          <div className="weather-location">{config.location}</div>
        )}
        <WeatherIcon code={weather.current.weatherCode} className="weather-icon" />
        <div className="weather-temp">
          {Math.round(weather.current.temperatureC)}
          {tempUnit(units)}
        </div>
        <div className="weather-label">{labelForCode(weather.current.weatherCode)}</div>
        <div className="weather-meta">
          Feels {Math.round(weather.current.apparentC)}
          {tempUnit(units)} · {weather.current.humidity}% · {Math.round(weather.current.windSpeed)} {speedUnit(units)}
        </div>
      </div>
      <div className="weather-daily">
        {weather.daily.slice(1, 4).map((d) => (
          <div className="daily-row" key={d.date}>
            <div className="daily-day">{dayLabel(d.date)}</div>
            <WeatherIcon code={d.weatherCode} className="daily-icon" />
            <div className="daily-range">
              {Math.round(d.minC)}° / {Math.round(d.maxC)}°
            </div>
          </div>
        ))}
      </div>
    </button>
  );
}

import type { Weather, WeatherSnapshot } from "../types";

interface Props {
  weather: WeatherSnapshot | null;
  config: Weather | undefined;
}

// https://open-meteo.com/en/docs#api_form - WMO weather codes
const CODE_MAP: Record<number, { label: string; icon: string }> = {
  0: { label: "Clear", icon: "☀" },
  1: { label: "Mainly clear", icon: "🌤" },
  2: { label: "Partly cloudy", icon: "⛅" },
  3: { label: "Overcast", icon: "☁" },
  45: { label: "Fog", icon: "🌫" },
  48: { label: "Rime fog", icon: "🌫" },
  51: { label: "Light drizzle", icon: "🌦" },
  53: { label: "Drizzle", icon: "🌦" },
  55: { label: "Heavy drizzle", icon: "🌧" },
  61: { label: "Light rain", icon: "🌧" },
  63: { label: "Rain", icon: "🌧" },
  65: { label: "Heavy rain", icon: "🌧" },
  71: { label: "Light snow", icon: "🌨" },
  73: { label: "Snow", icon: "🌨" },
  75: { label: "Heavy snow", icon: "❄" },
  80: { label: "Showers", icon: "🌦" },
  81: { label: "Showers", icon: "🌧" },
  82: { label: "Violent showers", icon: "⛈" },
  95: { label: "Thunderstorm", icon: "⛈" },
  96: { label: "T-storm w/ hail", icon: "⛈" },
  99: { label: "T-storm w/ hail", icon: "⛈" },
};

function describe(code: number): { label: string; icon: string } {
  return CODE_MAP[code] ?? { label: "—", icon: "·" };
}

function tempUnit(units: string | undefined): string {
  return units === "imperial" ? "°F" : "°C";
}

function speedUnit(units: string | undefined): string {
  return units === "imperial" ? "mph" : "km/h";
}

function dayLabel(iso: string): string {
  const d = new Date(iso + "T00:00:00");
  return d.toLocaleDateString([], { weekday: "short" });
}

export default function WeatherWidget({ weather, config }: Props) {
  if (!weather) {
    return (
      <div className="widget weather-widget">
        <div className="weather-empty">Weather unavailable</div>
      </div>
    );
  }
  const units = config?.units ?? weather.units;
  const cur = describe(weather.current.weatherCode);
  return (
    <div className="widget weather-widget">
      <div className="weather-current">
        {config?.location && (
          <div className="weather-location">{config.location}</div>
        )}
        <span className="weather-icon" aria-hidden>{cur.icon}</span>
        <div className="weather-temp">
          {Math.round(weather.current.temperatureC)}
          {tempUnit(units)}
        </div>
        <div className="weather-label">{cur.label}</div>
        <div className="weather-meta">
          Feels {Math.round(weather.current.apparentC)}
          {tempUnit(units)} · {weather.current.humidity}% · {Math.round(weather.current.windSpeed)} {speedUnit(units)}
        </div>
      </div>
      <div className="weather-daily">
        {weather.daily.slice(1, 4).map((d) => {
          const info = describe(d.weatherCode);
          return (
            <div className="daily-row" key={d.date}>
              <div className="daily-day">{dayLabel(d.date)}</div>
              <div className="daily-icon" aria-hidden>{info.icon}</div>
              <div className="daily-range">
                {Math.round(d.minC)}° / {Math.round(d.maxC)}°
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}

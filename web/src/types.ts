export interface Calendar {
  id: string;
  name: string;
  color: string;
  url: string;
}

export interface Weather {
  enabled: boolean;
  latitude: number;
  longitude: number;
  units: "metric" | "imperial";
  timezone: string;
  location: string;
}

export interface Tide {
  enabled: boolean;
  latitude: number;
  longitude: number;
  units: "metric" | "imperial";
  timezone: string;
  location: string;
}

export interface GeoResult {
  name: string;
  admin1?: string;
  country?: string;
  timezone?: string;
  latitude: number;
  longitude: number;
}

export type ThemePalette = "default" | "ocean" | "sunset" | "forest";
export type ThemeMode = "light" | "dark" | "auto";

export interface Display {
  defaultView: "day" | "week" | "month";
  calendarRefreshSeconds: number;
  weatherRefreshSeconds: number;
  tideRefreshSeconds: number;
  theme: ThemePalette;
  mode: ThemeMode;
  calendarEnabled: boolean;
  clockEnabled: boolean;
}

export interface SnowDay {
  enabled: boolean;
  url: string;
}

export interface Config {
  calendars: Calendar[];
  weather: Weather;
  tide: Tide;
  snowDay: SnowDay;
  display: Display;
}

export interface CalendarEvent {
  id: string;
  calendarId: string;
  calendarName: string;
  calendarColor: string;
  title: string;
  start: string;
  end: string;
  allDay: boolean;
  location?: string;
  description?: string;
}

export interface WeatherCurrent {
  time: string;
  temperatureC: number;
  apparentC: number;
  humidity: number;
  windSpeed: number;
  weatherCode: number;
  isDay: boolean;
  precipitation: number;
}

export interface WeatherDaily {
  date: string;
  maxC: number;
  minC: number;
  weatherCode: number;
  sunrise: string;
  sunset: string;
  precipMM: number;
  windSpeedMax: number;
}

export interface WeatherSnapshot {
  updatedAt: string;
  units: string;
  timezone: string;
  current: WeatherCurrent;
  daily: WeatherDaily[];
}

export interface TideEvent {
  time: string;
  type: "high" | "low";
  heightMeters: number;
}

export interface TideSnapshot {
  updatedAt: string;
  units: string;
  timezone: string;
  currentMeters: number;
  events: TideEvent[];
}

export interface SnowDaySnapshot {
  updatedAt: string;
  url: string;
  location: string;
  regionName: string;
  morningTime: string;
  probability: number;
  score: number;
  category: string;
}

export type WSFrame =
  | {
      type: "snapshot";
      config: Config;
      events: CalendarEvent[];
      weather: WeatherSnapshot | null;
      snowday: SnowDaySnapshot | null;
      tide: TideSnapshot | null;
    }
  | { type: "calendar"; events: CalendarEvent[] }
  | { type: "weather"; weather: WeatherSnapshot | null }
  | { type: "snowday"; snowday: SnowDaySnapshot | null }
  | { type: "tide"; tide: TideSnapshot | null }
  | { type: "config"; config: Config };

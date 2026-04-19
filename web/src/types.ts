export interface Calendar {
  id: string;
  name: string;
  color: string;
  url: string;
}

export interface Weather {
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
  theme: ThemePalette;
  mode: ThemeMode;
}

export interface SnowDay {
  url: string;
}

export interface Config {
  calendars: Calendar[];
  weather: Weather;
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
}

export interface WeatherSnapshot {
  updatedAt: string;
  units: string;
  timezone: string;
  current: WeatherCurrent;
  daily: WeatherDaily[];
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
    }
  | { type: "calendar"; events: CalendarEvent[] }
  | { type: "weather"; weather: WeatherSnapshot | null }
  | { type: "snowday"; snowday: SnowDaySnapshot | null }
  | { type: "config"; config: Config };

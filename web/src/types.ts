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
}

export interface Display {
  defaultView: "day" | "week" | "month";
  calendarRefreshSeconds: number;
  weatherRefreshSeconds: number;
  theme: "light" | "dark";
}

export interface Config {
  calendars: Calendar[];
  weather: Weather;
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

export type WSFrame =
  | { type: "snapshot"; config: Config; events: CalendarEvent[]; weather: WeatherSnapshot | null }
  | { type: "calendar"; events: CalendarEvent[] }
  | { type: "weather"; weather: WeatherSnapshot | null }
  | { type: "config"; config: Config };

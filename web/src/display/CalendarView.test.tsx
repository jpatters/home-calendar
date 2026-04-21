import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { cleanup, render } from "@testing-library/react";
import CalendarView from "./CalendarView";
import type { Weather, WeatherSnapshot } from "../types";

const TODAY = new Date("2026-04-22T12:00:00");

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(TODAY);
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

function dateKey(d: Date): string {
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

function daysFromToday(offset: number): string {
  const d = new Date(TODAY);
  d.setDate(d.getDate() + offset);
  return dateKey(d);
}

function makeWeather(
  overrides: {
    daily?: Array<{ date: string; maxC: number; weatherCode?: number }>;
  } = {},
): WeatherSnapshot {
  const daily = overrides.daily ?? [
    { date: dateKey(TODAY), maxC: 18, weatherCode: 0 },
  ];
  return {
    updatedAt: TODAY.toISOString(),
    units: "metric",
    timezone: "UTC",
    current: {
      time: TODAY.toISOString(),
      temperatureC: 15,
      apparentC: 14,
      humidity: 50,
      windSpeed: 10,
      weatherCode: 0,
      isDay: true,
      precipitation: 0,
    },
    daily: daily.map((d) => ({
      date: d.date,
      maxC: d.maxC,
      minC: 0,
      weatherCode: d.weatherCode ?? 0,
      sunrise: "",
      sunset: "",
      precipMM: 0,
    })),
  };
}

function makeWeatherConfig(overrides: Partial<Weather> = {}): Weather {
  return {
    enabled: overrides.enabled ?? true,
    latitude: 0,
    longitude: 0,
    units: overrides.units ?? "metric",
    timezone: "UTC",
    location: "Test",
  };
}

const noop = () => {};

describe("CalendarView weather in day header", () => {
  test("week view renders weather for each forecast day", () => {
    const weather = makeWeather({
      daily: [
        { date: daysFromToday(0), maxC: 16, weatherCode: 0 },
        { date: daysFromToday(1), maxC: 18, weatherCode: 1 },
        { date: daysFromToday(2), maxC: 20, weatherCode: 3 },
      ],
    });
    const { getAllByLabelText } = render(
      <CalendarView
        events={[]}
        defaultView="week"
        weather={weather}
        weatherConfig={makeWeatherConfig()}
        onEventClick={noop}
        onDayClick={noop}
      />,
    );
    const cells = getAllByLabelText(/high \d+°C/i);
    expect(cells.length).toBe(3);
  });

  test("high temperature is rounded and uses metric unit by default", () => {
    const weather = makeWeather({
      daily: [{ date: dateKey(TODAY), maxC: 22.7, weatherCode: 0 }],
    });
    const { getByLabelText } = render(
      <CalendarView
        events={[]}
        defaultView="day"
        weather={weather}
        weatherConfig={makeWeatherConfig({ units: "metric" })}
        onEventClick={noop}
        onDayClick={noop}
      />,
    );
    expect(getByLabelText(/high 23°C/i)).not.toBeNull();
  });

  test("high temperature uses imperial unit when configured", () => {
    const weather = makeWeather({
      daily: [{ date: dateKey(TODAY), maxC: 68.3, weatherCode: 0 }],
    });
    const { getByLabelText } = render(
      <CalendarView
        events={[]}
        defaultView="day"
        weather={weather}
        weatherConfig={makeWeatherConfig({ units: "imperial" })}
        onEventClick={noop}
        onDayClick={noop}
      />,
    );
    expect(getByLabelText(/high 68°F/i)).not.toBeNull();
  });

  test("no weather is rendered when snapshot is null", () => {
    const { queryAllByLabelText } = render(
      <CalendarView
        events={[]}
        defaultView="week"
        weather={null}
        weatherConfig={makeWeatherConfig()}
        onEventClick={noop}
        onDayClick={noop}
      />,
    );
    expect(queryAllByLabelText(/high \d+°/i).length).toBe(0);
  });

  test("no weather is rendered when weather is disabled", () => {
    const weather = makeWeather({
      daily: [{ date: dateKey(TODAY), maxC: 20, weatherCode: 0 }],
    });
    const { queryAllByLabelText } = render(
      <CalendarView
        events={[]}
        defaultView="week"
        weather={weather}
        weatherConfig={makeWeatherConfig({ enabled: false })}
        onEventClick={noop}
        onDayClick={noop}
      />,
    );
    expect(queryAllByLabelText(/high \d+°/i).length).toBe(0);
  });

  test("no weather is rendered for days outside the forecast window", () => {
    const weather = makeWeather({
      daily: [{ date: "2099-12-25", maxC: 20, weatherCode: 0 }],
    });
    const { queryAllByLabelText } = render(
      <CalendarView
        events={[]}
        defaultView="week"
        weather={weather}
        weatherConfig={makeWeatherConfig()}
        onEventClick={noop}
        onDayClick={noop}
      />,
    );
    expect(queryAllByLabelText(/high \d+°/i).length).toBe(0);
  });

  test("month view does not render weather even with matching data", () => {
    const weather = makeWeather({
      daily: [
        { date: daysFromToday(-1), maxC: 14, weatherCode: 1 },
        { date: daysFromToday(0), maxC: 16, weatherCode: 0 },
        { date: daysFromToday(1), maxC: 18, weatherCode: 2 },
      ],
    });
    const { queryAllByLabelText } = render(
      <CalendarView
        events={[]}
        defaultView="month"
        weather={weather}
        weatherConfig={makeWeatherConfig()}
        onEventClick={noop}
        onDayClick={noop}
      />,
    );
    expect(queryAllByLabelText(/high \d+°/i).length).toBe(0);
  });

  test("day view renders exactly one weather cell when data matches today", () => {
    const weather = makeWeather({
      daily: [{ date: dateKey(TODAY), maxC: 18, weatherCode: 1 }],
    });
    const { getAllByLabelText } = render(
      <CalendarView
        events={[]}
        defaultView="day"
        weather={weather}
        weatherConfig={makeWeatherConfig()}
        onEventClick={noop}
        onDayClick={noop}
      />,
    );
    expect(getAllByLabelText(/high \d+°/i).length).toBe(1);
  });
});

import { describe, expect, test } from "vitest";
import { resolveMode } from "./theme";
import type { WeatherSnapshot } from "./types";

function weatherWith(date: string, sunrise: string, sunset: string): WeatherSnapshot {
  return {
    updatedAt: "2026-04-19T00:00:00Z",
    units: "metric",
    timezone: "America/Toronto",
    current: {
      time: `${date}T12:00`,
      temperatureC: 15,
      apparentC: 15,
      humidity: 50,
      windSpeed: 5,
      weatherCode: 0,
      isDay: true,
      precipitation: 0,
    },
    daily: [
      {
        date,
        maxC: 20,
        minC: 5,
        weatherCode: 0,
        sunrise,
        sunset,
        precipMM: 0,
      },
    ],
  };
}

describe("resolveMode", () => {
  test("mode light returns light regardless of weather or time", () => {
    const now = new Date("2026-04-19T22:00:00");
    const weather = weatherWith("2026-04-19", "2026-04-19T06:00", "2026-04-19T20:00");
    expect(resolveMode("light", weather, now)).toBe("light");
  });

  test("mode dark returns dark regardless of weather or time", () => {
    const now = new Date("2026-04-19T12:00:00");
    const weather = weatherWith("2026-04-19", "2026-04-19T06:00", "2026-04-19T20:00");
    expect(resolveMode("dark", weather, now)).toBe("dark");
  });

  test("auto at midday returns light", () => {
    const now = new Date("2026-04-19T12:00:00");
    const weather = weatherWith("2026-04-19", "2026-04-19T06:00", "2026-04-19T20:00");
    expect(resolveMode("auto", weather, now)).toBe("light");
  });

  test("auto after sunset returns dark", () => {
    const now = new Date("2026-04-19T22:00:00");
    const weather = weatherWith("2026-04-19", "2026-04-19T06:00", "2026-04-19T20:00");
    expect(resolveMode("auto", weather, now)).toBe("dark");
  });

  test("auto before sunrise returns dark", () => {
    const now = new Date("2026-04-19T05:00:00");
    const weather = weatherWith("2026-04-19", "2026-04-19T06:00", "2026-04-19T20:00");
    expect(resolveMode("auto", weather, now)).toBe("dark");
  });

  test("auto with no weather snapshot falls back to light", () => {
    const now = new Date("2026-04-19T22:00:00");
    expect(resolveMode("auto", null, now)).toBe("light");
  });

  test("auto where today has no matching daily entry falls back to light", () => {
    const now = new Date("2026-04-19T22:00:00");
    const weather = weatherWith("2026-04-20", "2026-04-20T06:00", "2026-04-20T20:00");
    expect(resolveMode("auto", weather, now)).toBe("light");
  });

  test("auto at exact sunrise is light", () => {
    const now = new Date("2026-04-19T06:00:00");
    const weather = weatherWith("2026-04-19", "2026-04-19T06:00", "2026-04-19T20:00");
    expect(resolveMode("auto", weather, now)).toBe("light");
  });

  test("auto at exact sunset is dark", () => {
    const now = new Date("2026-04-19T20:00:00");
    const weather = weatherWith("2026-04-19", "2026-04-19T06:00", "2026-04-19T20:00");
    expect(resolveMode("auto", weather, now)).toBe("dark");
  });
});

import { afterEach, describe, expect, test } from "vitest";
import { cleanup, render } from "@testing-library/react";
import WeatherWidget from "./WeatherWidget";
import type { WeatherSnapshot } from "../types";

afterEach(() => {
  cleanup();
});

function makeSnapshot(
  overrides: { currentCode?: number; dailyCodes?: [number, number, number] } = {},
): WeatherSnapshot {
  const currentCode = overrides.currentCode ?? 0;
  const dailyCodes = overrides.dailyCodes ?? [1, 2, 3];
  return {
    updatedAt: "2026-01-01T00:00:00Z",
    units: "metric",
    timezone: "UTC",
    current: {
      time: "2026-01-01T00:00:00Z",
      temperatureC: 20,
      apparentC: 19,
      humidity: 55,
      windSpeed: 10,
      weatherCode: currentCode,
      isDay: true,
      precipitation: 0,
    },
    daily: [
      { date: "2026-01-01", maxC: 20, minC: 10, weatherCode: currentCode, sunrise: "", sunset: "", precipMM: 0 },
      { date: "2026-01-02", maxC: 22, minC: 12, weatherCode: dailyCodes[0], sunrise: "", sunset: "", precipMM: 0 },
      { date: "2026-01-03", maxC: 18, minC: 8, weatherCode: dailyCodes[1], sunrise: "", sunset: "", precipMM: 0 },
      { date: "2026-01-04", maxC: 15, minC: 5, weatherCode: dailyCodes[2], sunrise: "", sunset: "", precipMM: 0 },
    ],
  };
}

describe("WeatherWidget icons", () => {
  test("renders an svg for the current weather icon slot", () => {
    const snap = makeSnapshot({ currentCode: 0 });
    const { container } = render(<WeatherWidget weather={snap} config={undefined} />);
    const iconEl = container.querySelector(".weather-icon");
    expect(iconEl).not.toBeNull();
    expect(iconEl!.querySelector("svg")).not.toBeNull();
  });

  test("renders distinct icons for clearly different weather codes", () => {
    const clear = render(
      <WeatherWidget weather={makeSnapshot({ currentCode: 0 })} config={undefined} />,
    );
    const thunder = render(
      <WeatherWidget weather={makeSnapshot({ currentCode: 95 })} config={undefined} />,
    );
    const clearSvg = clear.container.querySelector(".weather-icon svg")?.outerHTML;
    const thunderSvg = thunder.container.querySelector(".weather-icon svg")?.outerHTML;
    expect(clearSvg).toBeTruthy();
    expect(thunderSvg).toBeTruthy();
    expect(clearSvg).not.toEqual(thunderSvg);
  });

  test("renders one svg per day in the 3-day daily forecast", () => {
    const snap = makeSnapshot({ dailyCodes: [1, 3, 65] });
    const { container } = render(<WeatherWidget weather={snap} config={undefined} />);
    const dailySvgs = container.querySelectorAll(".weather-daily svg");
    expect(dailySvgs.length).toBe(3);
  });

  test("does not render emoji weather characters anywhere in the widget", () => {
    const snap = makeSnapshot({ currentCode: 0, dailyCodes: [3, 65, 95] });
    const { container } = render(<WeatherWidget weather={snap} config={undefined} />);
    const text = container.textContent ?? "";
    const bannedChars = ["☀", "🌤", "⛅", "☁", "🌫", "🌦", "🌧", "🌨", "❄", "⛈"];
    for (const ch of bannedChars) {
      expect(text, `widget must not contain emoji ${ch}`).not.toContain(ch);
    }
  });

  test("renders an svg fallback for unknown weather codes", () => {
    const snap = makeSnapshot({ currentCode: 9999 });
    const { container } = render(<WeatherWidget weather={snap} config={undefined} />);
    const iconEl = container.querySelector(".weather-icon");
    expect(iconEl!.querySelector("svg")).not.toBeNull();
  });

  test("displays the human-readable label for known codes", () => {
    const snap = makeSnapshot({ currentCode: 0 });
    const { getByText } = render(<WeatherWidget weather={snap} config={undefined} />);
    expect(getByText("Clear")).not.toBeNull();
  });
});

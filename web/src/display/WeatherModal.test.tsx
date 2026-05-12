import { afterEach, describe, expect, test } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import WeatherModal from "./WeatherModal";
import type { Weather, WeatherSnapshot } from "../types";

afterEach(() => cleanup());

const metricConfig: Weather = {
  enabled: true,
  latitude: 43.65,
  longitude: -79.38,
  units: "metric",
  timezone: "America/Toronto",
  location: "Toronto, ON",
  ecowittUrl: "",
};

function stationFixture() {
  return {
    updatedAt: "2026-04-21T12:00:00Z",
    hasOutdoor: true,
    hasIndoor: true,
    indoorTempC: 21.5,
    indoorHumidity: 44,
    pressureHPa: 1014.0,
    windGust: 4.1,
    windDirection: 207,
    solarWM2: 120.0,
    rainRate: 0.4,
    rainEvent: 0.8,
    rainDaily: 2.5,
    rainWeekly: 7.0,
    rainMonthly: 30.0,
    rainYearly: 150.0,
  };
}

function buildSevenDaySnapshot(): WeatherSnapshot {
  return {
    updatedAt: "2026-04-21T12:00:00Z",
    units: "metric",
    timezone: "America/Toronto",
    current: {
      time: "2026-04-21T12:00:00Z",
      temperatureC: 14,
      apparentC: 13,
      humidity: 60,
      windSpeed: 11,
      weatherCode: 3,
      isDay: true,
      precipitation: 0,
    },
    daily: [
      { date: "2026-04-21", maxC: 18, minC: 8, weatherCode: 3,  sunrise: "2026-04-21T06:30", sunset: "2026-04-21T19:30", precipMM: 0,   windSpeedMax: 12 },
      { date: "2026-04-22", maxC: 19, minC: 9, weatherCode: 61, sunrise: "2026-04-22T06:29", sunset: "2026-04-22T19:31", precipMM: 1.2, windSpeedMax: 18.5 },
      { date: "2026-04-23", maxC: 20, minC: 10, weatherCode: 80, sunrise: "2026-04-23T06:28", sunset: "2026-04-23T19:32", precipMM: 5,   windSpeedMax: 25 },
      { date: "2026-04-24", maxC: 21, minC: 11, weatherCode: 95, sunrise: "2026-04-24T06:27", sunset: "2026-04-24T19:33", precipMM: 10,  windSpeedMax: 30 },
      { date: "2026-04-25", maxC: 22, minC: 12, weatherCode: 0,  sunrise: "2026-04-25T06:26", sunset: "2026-04-25T19:34", precipMM: 0,   windSpeedMax: 8 },
      { date: "2026-04-26", maxC: 23, minC: 13, weatherCode: 1,  sunrise: "2026-04-26T06:25", sunset: "2026-04-26T19:35", precipMM: 0,   windSpeedMax: 14 },
      { date: "2026-04-27", maxC: 24, minC: 14, weatherCode: 2,  sunrise: "2026-04-27T06:24", sunset: "2026-04-27T19:36", precipMM: 0,   windSpeedMax: 22 },
    ],
  };
}

// Helpers: simulate a touch gesture on an element.
function swipeLeft(el: Element) {
  fireEvent.touchStart(el, { touches: [{ clientX: 240, clientY: 100 }] });
  fireEvent.touchEnd(el,   { changedTouches: [{ clientX: 60,  clientY: 110 }] });
}
function swipeRight(el: Element) {
  fireEvent.touchStart(el, { touches: [{ clientX: 60,  clientY: 100 }] });
  fireEvent.touchEnd(el,   { changedTouches: [{ clientX: 240, clientY: 110 }] });
}
function swipeDown(el: Element) {
  fireEvent.touchStart(el, { touches: [{ clientX: 100, clientY: 50 }] });
  fireEvent.touchEnd(el,   { changedTouches: [{ clientX: 105, clientY: 300 }] });
}

describe("WeatherModal", () => {
  test("shows today's forecast (index 0) initially", () => {
    render(<WeatherModal weather={buildSevenDaySnapshot()} config={metricConfig} onClose={() => {}} />);
    // Day 0: max 18, min 8, wind 12. Use exact strings so "18 °C" doesn't
    // inadvertently match "8 °C" as a substring.
    expect(screen.getByText("18 °C")).toBeTruthy();
    expect(screen.getByText("8 °C")).toBeTruthy();
    expect(screen.getByText(/12\s*km\/h/)).toBeTruthy();
    // Day 1 max (19) should NOT be on screen
    expect(screen.queryByText("19 °C")).toBeNull();
  });

  test("next button advances to tomorrow", () => {
    render(<WeatherModal weather={buildSevenDaySnapshot()} config={metricConfig} onClose={() => {}} />);
    fireEvent.click(screen.getByRole("button", { name: /next day/i }));
    expect(screen.getByText("19 °C")).toBeTruthy();
    // Day 1 wind is 18.5 km/h in the fixture; the UI rounds to 19.
    expect(screen.getByText(/19\s*km\/h/)).toBeTruthy();
    expect(screen.queryByText("18 °C")).toBeNull();
  });

  test("prev button returns to previous day", () => {
    render(<WeatherModal weather={buildSevenDaySnapshot()} config={metricConfig} onClose={() => {}} />);
    fireEvent.click(screen.getByRole("button", { name: /next day/i }));
    fireEvent.click(screen.getByRole("button", { name: /previous day/i }));
    // Back to day 0
    expect(screen.getByText("18 °C")).toBeTruthy();
    expect(screen.queryByText("19 °C")).toBeNull();
  });

  test("prev is disabled on the first day", () => {
    render(<WeatherModal weather={buildSevenDaySnapshot()} config={metricConfig} onClose={() => {}} />);
    const prev = screen.getByRole("button", { name: /previous day/i });
    expect((prev as HTMLButtonElement).disabled).toBe(true);
  });

  test("next is disabled on the last day", () => {
    render(<WeatherModal weather={buildSevenDaySnapshot()} config={metricConfig} onClose={() => {}} />);
    const next = screen.getByRole("button", { name: /next day/i });
    // Advance six times to reach day 6.
    for (let i = 0; i < 6; i++) fireEvent.click(next);
    expect((next as HTMLButtonElement).disabled).toBe(true);
    // Day 6 content: max 24, wind 22
    expect(screen.getByText("24 °C")).toBeTruthy();
    expect(screen.getByText(/22\s*km\/h/)).toBeTruthy();
  });

  test("swiping left advances to the next day", () => {
    render(<WeatherModal weather={buildSevenDaySnapshot()} config={metricConfig} onClose={() => {}} />);
    swipeLeft(screen.getByRole("dialog"));
    expect(screen.getByText("19 °C")).toBeTruthy();
  });

  test("swiping right returns to the previous day", () => {
    render(<WeatherModal weather={buildSevenDaySnapshot()} config={metricConfig} onClose={() => {}} />);
    fireEvent.click(screen.getByRole("button", { name: /next day/i }));
    swipeRight(screen.getByRole("dialog"));
    expect(screen.getByText("18 °C")).toBeTruthy();
  });

  test("vertical drag does not change the day", () => {
    render(<WeatherModal weather={buildSevenDaySnapshot()} config={metricConfig} onClose={() => {}} />);
    swipeDown(screen.getByRole("dialog"));
    // Still showing day 0.
    expect(screen.getByText("18 °C")).toBeTruthy();
    expect(screen.queryByText("19 °C")).toBeNull();
  });

  test("close button invokes onClose", () => {
    let closed = 0;
    render(<WeatherModal weather={buildSevenDaySnapshot()} config={metricConfig} onClose={() => { closed += 1; }} />);
    fireEvent.click(screen.getByRole("button", { name: /close/i }));
    expect(closed).toBe(1);
  });

  test("backdrop click invokes onClose", () => {
    let closed = 0;
    render(<WeatherModal weather={buildSevenDaySnapshot()} config={metricConfig} onClose={() => { closed += 1; }} />);
    const dialog = screen.getByRole("dialog");
    const backdrop = dialog.parentElement!;
    fireEvent.click(backdrop);
    expect(closed).toBe(1);
  });

  test("imperial units render °F and mph", () => {
    render(
      <WeatherModal
        weather={buildSevenDaySnapshot()}
        config={{ ...metricConfig, units: "imperial" }}
        onClose={() => {}}
      />,
    );
    // The snapshot's numeric values don't change -- the server converts upstream --
    // we just assert the unit suffixes the user sees.
    expect(screen.getAllByText(/°F/).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/mph/).length).toBeGreaterThan(0);
    expect(screen.queryAllByText(/°C/).length).toBe(0);
  });

  test("does not render a Live Station section when station is null", () => {
    render(<WeatherModal weather={buildSevenDaySnapshot()} config={metricConfig} onClose={() => {}} />);
    expect(screen.queryByText(/live station/i)).toBeNull();
    expect(screen.queryByText(/pressure/i)).toBeNull();
  });

  test("appends a Live Station page after the daily forecast when station is set", () => {
    const snap = { ...buildSevenDaySnapshot(), station: stationFixture() };
    render(<WeatherModal weather={snap} config={metricConfig} onClose={() => {}} />);
    const next = screen.getByRole("button", { name: /next day/i });
    // 7 days + 1 station = 8 pages, so 7 clicks to reach the station page.
    for (let i = 0; i < 7; i++) fireEvent.click(next);
    expect(screen.getByRole("heading", { name: /live station/i })).toBeTruthy();
    expect((next as HTMLButtonElement).disabled).toBe(true);
  });

  test("Live Station page surfaces indoor temp, humidity, pressure, gust, direction, solar, rain totals", () => {
    const snap = { ...buildSevenDaySnapshot(), station: stationFixture() };
    render(<WeatherModal weather={snap} config={metricConfig} onClose={() => {}} />);
    const next = screen.getByRole("button", { name: /next day/i });
    for (let i = 0; i < 7; i++) fireEvent.click(next);
    const body = document.body.textContent ?? "";
    expect(body).toMatch(/21\.5\s*°C/); // indoor temp
    expect(body).toMatch(/44\s*%/); // indoor humidity
    expect(body).toMatch(/1014\.0\s*hPa/); // pressure
    expect(body).toMatch(/207°/); // wind direction degrees
    expect(body).toMatch(/SSW/); // compass label for 207°
    expect(body).toMatch(/120\.0\s*W\/m²/); // solar
    expect(body).toMatch(/2\.50\s*mm/); // rain daily
    expect(body).toMatch(/150\.0\s*mm/); // rain yearly
  });

  test("header shows Today / Tomorrow / weekday labels", () => {
    render(<WeatherModal weather={buildSevenDaySnapshot()} config={metricConfig} onClose={() => {}} />);
    expect(screen.getByRole("heading", { name: /today/i })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /next day/i }));
    expect(screen.getByRole("heading", { name: /tomorrow/i })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /next day/i }));
    // Day 2: should be a weekday label, not "Today" or "Tomorrow"
    const heading = screen.getByRole("heading", { level: 2 });
    expect(/today/i.test(heading.textContent ?? "")).toBe(false);
    expect(/tomorrow/i.test(heading.textContent ?? "")).toBe(false);
  });
});

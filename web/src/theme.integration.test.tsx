import { afterEach, describe, expect, test, vi } from "vitest";
import { render, cleanup } from "@testing-library/react";
import { useTheme } from "./theme";
import type { LiveData } from "./useLiveData";
import type { Config, WeatherSnapshot } from "./types";

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  document.documentElement.removeAttribute("data-palette");
  document.documentElement.removeAttribute("data-mode");
});

function buildLive(theme: Config["display"]["theme"], mode: Config["display"]["mode"], weather: WeatherSnapshot | null = null): LiveData {
  return {
    ready: true,
    connected: true,
    events: [],
    snowday: null,
    tide: null,
    weather,
    config: {
      calendars: [],
      weather: {
        latitude: 0,
        longitude: 0,
        units: "metric",
        timezone: "UTC",
        location: "",
      },
      tide: {
        latitude: 0,
        longitude: 0,
        units: "metric",
        timezone: "UTC",
        location: "",
      },
      snowDay: { url: "" },
      display: {
        defaultView: "week",
        calendarRefreshSeconds: 300,
        weatherRefreshSeconds: 900,
        tideRefreshSeconds: 3600,
        theme,
        mode,
      },
    },
  };
}

function Harness({ live }: { live: LiveData }) {
  useTheme(live);
  return null;
}

describe("useTheme", () => {
  test("applies palette and light mode to document element", () => {
    const live = buildLive("ocean", "light");
    render(<Harness live={live} />);
    expect(document.documentElement.dataset.palette).toBe("ocean");
    expect(document.documentElement.dataset.mode).toBe("light");
  });

  test("applies palette and dark mode to document element", () => {
    const live = buildLive("forest", "dark");
    render(<Harness live={live} />);
    expect(document.documentElement.dataset.palette).toBe("forest");
    expect(document.documentElement.dataset.mode).toBe("dark");
  });

  test("auto mode resolves to dark after sunset", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-04-19T22:00:00"));
    const weather: WeatherSnapshot = {
      updatedAt: "2026-04-19T00:00:00Z",
      units: "metric",
      timezone: "America/Toronto",
      current: {
        time: "2026-04-19T22:00",
        temperatureC: 10,
        apparentC: 10,
        humidity: 50,
        windSpeed: 5,
        weatherCode: 0,
        isDay: false,
        precipitation: 0,
      },
      daily: [
        {
          date: "2026-04-19",
          maxC: 20,
          minC: 5,
          weatherCode: 0,
          sunrise: "2026-04-19T06:00",
          sunset: "2026-04-19T20:00",
          precipMM: 0,
        },
      ],
    };
    const live = buildLive("sunset", "auto", weather);
    render(<Harness live={live} />);
    expect(document.documentElement.dataset.palette).toBe("sunset");
    expect(document.documentElement.dataset.mode).toBe("dark");
  });

  test("does nothing when config is not yet loaded", () => {
    const live: LiveData = {
      ready: false,
      connected: false,
      config: null,
      events: [],
      weather: null,
      snowday: null,
      tide: null,
    };
    render(<Harness live={live} />);
    expect(document.documentElement.dataset.palette).toBeUndefined();
    expect(document.documentElement.dataset.mode).toBeUndefined();
  });

  test("updates document attributes when config changes", () => {
    const first = buildLive("default", "light");
    const { rerender } = render(<Harness live={first} />);
    expect(document.documentElement.dataset.palette).toBe("default");
    expect(document.documentElement.dataset.mode).toBe("light");

    const second = buildLive("forest", "dark");
    rerender(<Harness live={second} />);
    expect(document.documentElement.dataset.palette).toBe("forest");
    expect(document.documentElement.dataset.mode).toBe("dark");
  });
});

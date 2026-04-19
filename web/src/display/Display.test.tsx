import { afterEach, describe, expect, test, vi } from "vitest";
import { cleanup, render } from "@testing-library/react";
import Display from "./Display";
import type { LiveData } from "../useLiveData";
import type { Config } from "../types";

vi.mock("./CalendarView", () => ({
  default: () => <div data-testid="calendar-view" />,
}));

afterEach(() => {
  cleanup();
});

function buildConfig(
  overrides: Partial<{
    weatherEnabled: boolean;
    tideEnabled: boolean;
    snowDayEnabled: boolean;
    calendarEnabled: boolean;
    clockEnabled: boolean;
  }> = {},
): Config {
  return {
    calendars: [],
    weather: {
      latitude: 0,
      longitude: 0,
      units: "metric",
      timezone: "UTC",
      location: "Test",
      enabled: overrides.weatherEnabled ?? true,
    },
    tide: {
      latitude: 0,
      longitude: 0,
      units: "metric",
      timezone: "UTC",
      location: "Test",
      enabled: overrides.tideEnabled ?? true,
    },
    snowDay: {
      url: "https://example.com",
      enabled: overrides.snowDayEnabled ?? true,
    },
    display: {
      defaultView: "week",
      calendarRefreshSeconds: 300,
      weatherRefreshSeconds: 900,
      tideRefreshSeconds: 3600,
      theme: "default",
      mode: "light",
      calendarEnabled: overrides.calendarEnabled ?? true,
      clockEnabled: overrides.clockEnabled ?? true,
    },
  };
}

function buildLive(config: Config): LiveData {
  return {
    ready: true,
    connected: true,
    config,
    events: [],
    weather: null,
    snowday: null,
    tide: null,
  };
}

describe("Display widget enable/disable", () => {
  test("weather widget is hidden when weather.enabled is false", () => {
    const live = buildLive(buildConfig({ weatherEnabled: false }));
    const { container } = render(<Display live={live} />);
    expect(container.querySelector(".weather-widget")).toBeNull();
  });

  test("weather widget is rendered when weather.enabled is true", () => {
    const live = buildLive(buildConfig({ weatherEnabled: true }));
    const { container } = render(<Display live={live} />);
    expect(container.querySelector(".weather-widget")).not.toBeNull();
  });

  test("tide widget is hidden when tide.enabled is false", () => {
    const live = buildLive(buildConfig({ tideEnabled: false }));
    const { container } = render(<Display live={live} />);
    expect(container.querySelector(".tide-widget")).toBeNull();
  });

  test("tide widget is rendered when tide.enabled is true", () => {
    const live = buildLive(buildConfig({ tideEnabled: true }));
    const { container } = render(<Display live={live} />);
    expect(container.querySelector(".tide-widget")).not.toBeNull();
  });

  test("snowday widget is hidden when snowDay.enabled is false", () => {
    const live = buildLive(buildConfig({ snowDayEnabled: false }));
    const { container } = render(<Display live={live} />);
    expect(container.querySelector(".snowday-widget")).toBeNull();
  });

  test("snowday widget is rendered when snowDay.enabled is true", () => {
    const live = buildLive(buildConfig({ snowDayEnabled: true }));
    const { container } = render(<Display live={live} />);
    expect(container.querySelector(".snowday-widget")).not.toBeNull();
  });

  test("clock widget is hidden when display.clockEnabled is false", () => {
    const live = buildLive(buildConfig({ clockEnabled: false }));
    const { container } = render(<Display live={live} />);
    expect(container.querySelector(".clock-widget")).toBeNull();
  });

  test("clock widget is rendered when display.clockEnabled is true", () => {
    const live = buildLive(buildConfig({ clockEnabled: true }));
    const { container } = render(<Display live={live} />);
    expect(container.querySelector(".clock-widget")).not.toBeNull();
  });

  test("calendar pane is hidden when display.calendarEnabled is false", () => {
    const live = buildLive(buildConfig({ calendarEnabled: false }));
    const { container } = render(<Display live={live} />);
    expect(container.querySelector(".calendar-pane")).toBeNull();
  });

  test("calendar pane is rendered when display.calendarEnabled is true", () => {
    const live = buildLive(buildConfig({ calendarEnabled: true }));
    const { container } = render(<Display live={live} />);
    expect(container.querySelector(".calendar-pane")).not.toBeNull();
  });
});

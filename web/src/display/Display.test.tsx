import { afterEach, describe, expect, test, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import Display from "./Display";
import type { LiveData } from "../useLiveData";
import type { Config } from "../types";

vi.mock("./CalendarView", () => ({
  default: () => <div data-testid="calendar-view" />,
}));

vi.mock("../browser", () => ({
  reloadPage: vi.fn(),
}));

import { reloadPage } from "../browser";

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function buildConfig(
  overrides: Partial<{
    weatherEnabled: boolean;
    tideEnabled: boolean;
    snowDayEnabled: boolean;
    baseballEnabled: boolean;
    poolEnabled: boolean;
    hotTubEnabled: boolean;
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
      stationCode: "01710",
      units: "metric",
      location: "Test",
      enabled: overrides.tideEnabled ?? true,
    },
    snowDay: {
      url: "https://example.com",
      enabled: overrides.snowDayEnabled ?? true,
    },
    baseball: {
      enabled: overrides.baseballEnabled ?? true,
      teamId: 0,
      teamName: "",
      teamAbbr: "",
    },
    pool: {
      enabled: overrides.poolEnabled ?? true,
      deviceUrl: "http://pool.test",
    },
    hotTub: {
      enabled: overrides.hotTubEnabled ?? true,
      host: "192.168.1.50",
    },
    display: {
      defaultView: "week",
      calendarRefreshSeconds: 300,
      weatherRefreshSeconds: 900,
      tideRefreshSeconds: 3600,
      baseballRefreshSeconds: 600,
      poolRefreshSeconds: 30,
      hotTubRefreshSeconds: 30,
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
    baseball: null,
    pool: null,
    hottub: null,
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

  test("pool widget is hidden when pool.enabled is false", () => {
    const live = buildLive(buildConfig({ poolEnabled: false }));
    const { container } = render(<Display live={live} />);
    expect(container.querySelector(".pool-widget")).toBeNull();
  });

  test("pool widget is rendered when pool.enabled is true", () => {
    const live = buildLive(buildConfig({ poolEnabled: true }));
    const { container } = render(<Display live={live} />);
    expect(container.querySelector(".pool-widget")).not.toBeNull();
  });

  test("hot tub widget is hidden when hotTub.enabled is false", () => {
    const live = buildLive(buildConfig({ hotTubEnabled: false }));
    const { container } = render(<Display live={live} />);
    expect(container.querySelector(".hottub-widget")).toBeNull();
  });

  test("hot tub widget is rendered when hotTub.enabled is true", () => {
    const live = buildLive(buildConfig({ hotTubEnabled: true }));
    const { container } = render(<Display live={live} />);
    expect(container.querySelector(".hottub-widget")).not.toBeNull();
  });
});

describe("Display hot tub modal", () => {
  test("tapping the hot tub widget opens the hot tub dialog", () => {
    const live = {
      ...buildLive(buildConfig({ hotTubEnabled: true })),
      hottub: {
        updatedAt: "2026-09-10T12:00:00Z",
        temperatureF: 67,
        targetF: 102,
        minTargetF: 59,
        maxTargetF: 106,
        heating: true,
      },
    };
    render(<Display live={live} />);
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: /hot tub details/i }));
    expect(screen.getByRole("dialog", { name: /hot tub/i })).toBeTruthy();
  });
});

describe("Display refresh button", () => {
  test("reloads the page when the refresh button is pressed", () => {
    const live = buildLive(buildConfig());
    render(<Display live={live} />);
    fireEvent.click(screen.getByRole("button", { name: /refresh/i }));
    expect(reloadPage).toHaveBeenCalledTimes(1);
  });

  test("reloads the page even while the live connection is down", () => {
    const live = { ...buildLive(buildConfig()), connected: false };
    render(<Display live={live} />);
    fireEvent.click(screen.getByRole("button", { name: /refresh/i }));
    expect(reloadPage).toHaveBeenCalledTimes(1);
  });
});

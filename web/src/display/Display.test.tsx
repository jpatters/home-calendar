import { afterEach, describe, expect, test, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import Display from "./Display";
import { buildConfig, buildLive } from "./displayFixtures";

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

  test("hockey widget is hidden when hockey.enabled is false", () => {
    const live = buildLive(buildConfig({ hockeyEnabled: false }));
    const { container } = render(<Display live={live} />);
    expect(container.querySelector(".hockey-widget")).toBeNull();
  });

  test("hockey widget is rendered when hockey.enabled is true", () => {
    const live = buildLive(buildConfig({ hockeyEnabled: true }));
    const { container } = render(<Display live={live} />);
    expect(container.querySelector(".hockey-widget")).not.toBeNull();
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

describe("Display pool modal", () => {
  test("tapping the pool widget opens the pool dialog", () => {
    const live = {
      ...buildLive(buildConfig({ poolEnabled: true })),
      pool: {
        updatedAt: "2026-08-06T12:00:00Z",
        temperatureC: 25.2,
        targetC: 27,
        minTargetC: 10,
        maxTargetC: 40,
        heating: true,
      },
    };
    render(<Display live={live} />);
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: /pool details/i }));
    expect(screen.getByRole("dialog", { name: /pool/i })).toBeTruthy();
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

import { afterEach, describe, expect, test } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import HotTubWidget from "./HotTubWidget";
import type { HotTubSnapshot } from "../types";

afterEach(() => cleanup());

function snapshot(overrides: Partial<HotTubSnapshot> = {}): HotTubSnapshot {
  return {
    updatedAt: "2026-09-10T12:00:00Z",
    temperatureF: 67,
    targetF: 102,
    heating: true,
    ...overrides,
  };
}

describe("HotTubWidget", () => {
  test("shows unavailable copy when snapshot is null", () => {
    render(<HotTubWidget hottub={null} />);
    expect(screen.getByText(/hot tub unavailable/i)).toBeTruthy();
  });

  test("shows the current water temperature in whole degrees Fahrenheit", () => {
    render(<HotTubWidget hottub={snapshot({ temperatureF: 67.6 })} />);
    expect(screen.getByText("68°F")).toBeTruthy();
  });

  test("indicates heating when the heater is on", () => {
    render(<HotTubWidget hottub={snapshot({ heating: true })} />);
    expect(screen.getByText(/heating/i)).toBeTruthy();
    expect(screen.queryByText(/^idle$/i)).toBeNull();
  });

  test("indicates idle when the heater is off", () => {
    render(<HotTubWidget hottub={snapshot({ heating: false })} />);
    expect(screen.getByText(/idle/i)).toBeTruthy();
    expect(screen.queryByText(/heating/i)).toBeNull();
  });

  test("shows the target temperature", () => {
    render(<HotTubWidget hottub={snapshot({ targetF: 102 })} />);
    expect(screen.getByText(/target 102°F/i)).toBeTruthy();
  });
});

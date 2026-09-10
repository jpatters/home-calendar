import { afterEach, describe, expect, test } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import HotTubWidget from "./HotTubWidget";
import type { HotTubSnapshot } from "../types";

afterEach(() => cleanup());

function snapshot(overrides: Partial<HotTubSnapshot> = {}): HotTubSnapshot {
  return {
    updatedAt: "2026-09-10T12:00:00Z",
    temperatureF: 67,
    targetF: 102,
    minTargetF: 59,
    maxTargetF: 106,
    heating: true,
    ...overrides,
  };
}

describe("HotTubWidget", () => {
  test("shows unavailable copy when snapshot is null", () => {
    render(<HotTubWidget hottub={null} onOpen={() => {}} />);
    expect(screen.getByText(/hot tub unavailable/i)).toBeTruthy();
  });

  test("shows the current water temperature in whole degrees Fahrenheit", () => {
    render(<HotTubWidget hottub={snapshot({ temperatureF: 67.6 })} onOpen={() => {}} />);
    expect(screen.getByText("68°F")).toBeTruthy();
  });

  test("indicates heating when the heater is on", () => {
    render(<HotTubWidget hottub={snapshot({ heating: true })} onOpen={() => {}} />);
    expect(screen.getByText(/heating/i)).toBeTruthy();
    expect(screen.queryByText(/^idle$/i)).toBeNull();
  });

  test("indicates idle when the heater is off", () => {
    render(<HotTubWidget hottub={snapshot({ heating: false })} onOpen={() => {}} />);
    expect(screen.getByText(/idle/i)).toBeTruthy();
    expect(screen.queryByText(/heating/i)).toBeNull();
  });

  test("shows the target temperature", () => {
    render(<HotTubWidget hottub={snapshot({ targetF: 102 })} onOpen={() => {}} />);
    expect(screen.getByText(/target 102°F/i)).toBeTruthy();
  });

  test("calls onOpen when tapped", () => {
    let opened = 0;
    render(<HotTubWidget hottub={snapshot()} onOpen={() => { opened += 1; }} />);
    fireEvent.click(screen.getByRole("button", { name: /hot tub details/i }));
    expect(opened).toBe(1);
  });

  test("offers nothing to tap while the hot tub is unavailable", () => {
    render(<HotTubWidget hottub={null} onOpen={() => {}} />);
    expect(screen.queryByRole("button")).toBeNull();
  });
});

import { afterEach, describe, expect, test } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import PoolWidget from "./PoolWidget";
import type { PoolSnapshot } from "../types";

afterEach(() => cleanup());

function snapshot(overrides: Partial<PoolSnapshot> = {}): PoolSnapshot {
  return {
    updatedAt: "2026-08-06T12:00:00Z",
    temperatureC: 25.2,
    targetC: 27.0,
    minTargetC: 10,
    maxTargetC: 40,
    heating: true,
    ...overrides,
  };
}

describe("PoolWidget", () => {
  test("shows unavailable copy when snapshot is null", () => {
    render(<PoolWidget pool={null} onOpen={() => {}} />);
    expect(screen.getByText(/pool unavailable/i)).toBeTruthy();
  });

  test("shows the current water temperature", () => {
    render(<PoolWidget pool={snapshot()} onOpen={() => {}} />);
    expect(screen.getByText(/25\.2/)).toBeTruthy();
  });

  test("indicates heating when the heat pump is calling for heat", () => {
    render(<PoolWidget pool={snapshot({ heating: true })} onOpen={() => {}} />);
    expect(screen.getByText(/heating/i)).toBeTruthy();
    expect(screen.queryByText(/^idle$/i)).toBeNull();
  });

  test("indicates idle when the heat pump is not heating", () => {
    render(<PoolWidget pool={snapshot({ heating: false })} onOpen={() => {}} />);
    expect(screen.getByText(/idle/i)).toBeTruthy();
    expect(screen.queryByText(/heating/i)).toBeNull();
  });

  test("shows the target temperature", () => {
    render(<PoolWidget pool={snapshot({ targetC: 27 })} onOpen={() => {}} />);
    expect(screen.getByText(/target/i)).toBeTruthy();
    expect(screen.getByText(/27\.0/)).toBeTruthy();
  });

  test("hides the target line when there is no setpoint", () => {
    render(<PoolWidget pool={snapshot({ targetC: 0 })} onOpen={() => {}} />);
    expect(screen.queryByText(/target/i)).toBeNull();
  });

  test("calls onOpen when tapped", () => {
    let opened = 0;
    render(<PoolWidget pool={snapshot()} onOpen={() => { opened += 1; }} />);
    fireEvent.click(screen.getByRole("button", { name: /pool details/i }));
    expect(opened).toBe(1);
  });

  test("offers nothing to tap while the pool is unavailable", () => {
    render(<PoolWidget pool={null} onOpen={() => {}} />);
    expect(screen.queryByRole("button")).toBeNull();
  });
});

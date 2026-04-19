import { afterEach, describe, expect, test } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import TideWidget from "./TideWidget";
import type { Tide, TideSnapshot } from "../types";

afterEach(() => cleanup());

const config: Tide = {
  latitude: 48.42,
  longitude: -123.36,
  units: "metric",
  timezone: "America/Vancouver",
  location: "Victoria, BC",
};

function snapshotWith(events: TideSnapshot["events"]): TideSnapshot {
  return {
    updatedAt: "2026-04-19T12:00:00Z",
    units: "metric",
    timezone: "America/Vancouver",
    currentMeters: 1.0,
    events,
  };
}

describe("TideWidget", () => {
  test("shows unavailable copy when snapshot is null", () => {
    render(<TideWidget tide={null} config={config} onOpen={() => {}} />);
    expect(screen.getByText(/tide unavailable/i)).toBeTruthy();
  });

  test("renders up to four upcoming tide events with type and metric height", () => {
    const events = [
      { time: "2026-04-19T15:42:00Z", type: "high" as const, heightMeters: 1.5 },
      { time: "2026-04-19T21:13:00Z", type: "low" as const, heightMeters: 0.3 },
      { time: "2026-04-20T04:01:00Z", type: "high" as const, heightMeters: 1.4 },
      { time: "2026-04-20T10:22:00Z", type: "low" as const, heightMeters: 0.2 },
      { time: "2026-04-20T16:00:00Z", type: "high" as const, heightMeters: 1.6 },
    ];
    render(
      <TideWidget
        tide={snapshotWith(events)}
        config={config}
        onOpen={() => {}}
      />,
    );
    // First four shown. Heights rendered in metres with 1-decimal precision.
    expect(screen.getAllByText(/high/i).length).toBeGreaterThanOrEqual(2);
    expect(screen.getAllByText(/low/i).length).toBeGreaterThanOrEqual(2);
    expect(screen.getByText(/1\.5\s*m\b/)).toBeTruthy();
    expect(screen.getByText(/0\.3\s*m\b/)).toBeTruthy();
    expect(screen.getByText(/1\.4\s*m\b/)).toBeTruthy();
    expect(screen.getByText(/0\.2\s*m\b/)).toBeTruthy();
    // Fifth event should NOT appear (widget limit is 4).
    expect(screen.queryByText(/1\.6\s*m\b/)).toBeNull();
  });

  test("renders heights in feet when units are imperial", () => {
    const events = [
      { time: "2026-04-19T15:42:00Z", type: "high" as const, heightMeters: 1.5 },
    ];
    render(
      <TideWidget
        tide={snapshotWith(events)}
        config={{ ...config, units: "imperial" }}
        onOpen={() => {}}
      />,
    );
    // 1.5 m ≈ 4.9 ft (rounded to 1 decimal).
    expect(screen.getByText(/4\.9\s*ft\b/)).toBeTruthy();
    expect(screen.queryByText(/1\.5\s*m\b/)).toBeNull();
  });

  test("invokes onOpen when the widget is activated", () => {
    let called = 0;
    const events = [
      { time: "2026-04-19T15:42:00Z", type: "high" as const, heightMeters: 1.5 },
    ];
    render(
      <TideWidget
        tide={snapshotWith(events)}
        config={config}
        onOpen={() => {
          called += 1;
        }}
      />,
    );
    fireEvent.click(
      screen.getByRole("button", { name: /tide details|tide information/i }),
    );
    expect(called).toBe(1);
  });
});

import { afterEach, describe, expect, test } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import TideModal from "./TideModal";
import type { Tide, TideSnapshot } from "../types";

afterEach(() => cleanup());

const config: Tide = {
  enabled: true,
  latitude: 48.42,
  longitude: -123.36,
  units: "metric",
  timezone: "America/Vancouver",
  location: "Victoria, BC",
};

function buildWeekSnapshot(): TideSnapshot {
  // Two events per day for 7 days = 14 events. Each day's height is distinct so
  // tests can identify which day's page is currently visible.
  const days = [
    "2026-04-19",
    "2026-04-20",
    "2026-04-21",
    "2026-04-22",
    "2026-04-23",
    "2026-04-24",
    "2026-04-25",
  ];
  const events = days.flatMap((d, i) => [
    { time: `${d}T06:00:00Z`, type: "high" as const, heightMeters: 1.3 + i * 0.1 },
    { time: `${d}T18:00:00Z`, type: "low" as const,  heightMeters: 0.2 + i * 0.05 },
  ]);
  return {
    updatedAt: "2026-04-19T00:00:00Z",
    units: "metric",
    timezone: "UTC",
    currentMeters: 0.8,
    events,
  };
}

function swipeLeft(el: Element) {
  fireEvent.touchStart(el, { touches: [{ clientX: 240, clientY: 100 }] });
  fireEvent.touchEnd(el,   { changedTouches: [{ clientX: 60,  clientY: 110 }] });
}
function swipeRight(el: Element) {
  fireEvent.touchStart(el, { touches: [{ clientX: 60,  clientY: 100 }] });
  fireEvent.touchEnd(el,   { changedTouches: [{ clientX: 240, clientY: 110 }] });
}

describe("TideModal", () => {
  test("shows only the first day's events initially", () => {
    render(<TideModal tide={buildWeekSnapshot()} config={config} onClose={() => {}} />);
    // Day 0 high is 1.3 m, day 1 high is 1.4 m.
    expect(screen.getByText(/1\.3\s*m\b/)).toBeTruthy();
    expect(screen.queryByText(/1\.4\s*m\b/)).toBeNull();
  });

  test("next button advances to the next day's events", () => {
    render(<TideModal tide={buildWeekSnapshot()} config={config} onClose={() => {}} />);
    fireEvent.click(screen.getByRole("button", { name: /next day/i }));
    expect(screen.getByText(/1\.4\s*m\b/)).toBeTruthy();
    expect(screen.queryByText(/1\.3\s*m\b/)).toBeNull();
  });

  test("prev button returns to the previous day", () => {
    render(<TideModal tide={buildWeekSnapshot()} config={config} onClose={() => {}} />);
    fireEvent.click(screen.getByRole("button", { name: /next day/i }));
    fireEvent.click(screen.getByRole("button", { name: /previous day/i }));
    expect(screen.getByText(/1\.3\s*m\b/)).toBeTruthy();
  });

  test("prev is disabled on the first day, next is disabled on the last day", () => {
    render(<TideModal tide={buildWeekSnapshot()} config={config} onClose={() => {}} />);
    const prev = screen.getByRole("button", { name: /previous day/i }) as HTMLButtonElement;
    const next = screen.getByRole("button", { name: /next day/i }) as HTMLButtonElement;
    expect(prev.disabled).toBe(true);
    // Advance to last day (index 6).
    for (let i = 0; i < 6; i++) fireEvent.click(next);
    expect(next.disabled).toBe(true);
    // Day 6: height starts at 1.3 + 6*0.1 = 1.9 m.
    expect(screen.getByText(/1\.9\s*m\b/)).toBeTruthy();
  });

  test("swiping left advances to the next day", () => {
    render(<TideModal tide={buildWeekSnapshot()} config={config} onClose={() => {}} />);
    swipeLeft(screen.getByRole("dialog"));
    expect(screen.getByText(/1\.4\s*m\b/)).toBeTruthy();
    expect(screen.queryByText(/1\.3\s*m\b/)).toBeNull();
  });

  test("swiping right returns to the previous day", () => {
    render(<TideModal tide={buildWeekSnapshot()} config={config} onClose={() => {}} />);
    fireEvent.click(screen.getByRole("button", { name: /next day/i }));
    swipeRight(screen.getByRole("dialog"));
    expect(screen.getByText(/1\.3\s*m\b/)).toBeTruthy();
  });

  test("calls onClose when the close button is clicked", () => {
    let closed = 0;
    render(
      <TideModal
        tide={buildWeekSnapshot()}
        config={config}
        onClose={() => {
          closed += 1;
        }}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: /close/i }));
    expect(closed).toBe(1);
  });

  test("calls onClose when the area outside the dialog is clicked", () => {
    let closed = 0;
    render(
      <TideModal
        tide={buildWeekSnapshot()}
        config={config}
        onClose={() => {
          closed += 1;
        }}
      />,
    );
    const dialog = screen.getByRole("dialog");
    const backdrop = dialog.parentElement;
    if (!backdrop) throw new Error("dialog has no parent backdrop");
    fireEvent.click(backdrop);
    expect(closed).toBe(1);
  });
});

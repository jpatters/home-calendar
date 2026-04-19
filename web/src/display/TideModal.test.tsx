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
  // Two events per day for 7 days = 14 events.
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
    { time: `${d}T18:00:00Z`, type: "low" as const, heightMeters: 0.2 + i * 0.05 },
  ]);
  return {
    updatedAt: "2026-04-19T00:00:00Z",
    units: "metric",
    timezone: "UTC",
    currentMeters: 0.8,
    events,
  };
}

describe("TideModal", () => {
  test("renders one section per day covering a week", () => {
    const snap = buildWeekSnapshot();
    render(<TideModal tide={snap} config={config} onClose={() => {}} />);
    // One <h3> day-heading per day covered.
    expect(screen.getAllByRole("heading", { level: 3 })).toHaveLength(7);
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

import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import TidePanel from "./TidePanel";
import type { Tide } from "../types";

function baseConfig(): Tide {
  return {
    enabled: true,
    stationCode: "",
    units: "metric",
    timezone: "America/Halifax",
    location: "",
  };
}

const stationResponse = [{ code: "01710", name: "Canoe Cove" }];

// Only the station directory answers. Anything else 404s, so searching the
// wrong endpoint surfaces as no results rather than passing quietly.
beforeEach(() => {
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string) =>
      Promise.resolve(
        String(url).includes("/api/tide/stations")
          ? ({
              ok: true,
              status: 200,
              json: () => Promise.resolve(stationResponse),
            } as unknown as Response)
          : ({
              ok: false,
              status: 404,
              json: () => Promise.resolve([]),
            } as unknown as Response),
      ),
    ),
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
  cleanup();
});

describe("TidePanel", () => {
  test("typing a place name, picking a station, surfaces its code and name via onChange", async () => {
    let captured: Tide | null = null;
    render(
      <TidePanel
        value={baseConfig()}
        onChange={(t) => {
          captured = t;
        }}
      />,
    );

    fireEvent.change(screen.getByPlaceholderText(/station/i), {
      target: { value: "canoe" },
    });

    const button = await screen.findByRole("button", { name: /canoe cove/i });
    fireEvent.click(button);

    expect(captured).toBeTruthy();
    expect(captured!.stationCode).toBe("01710");
    expect(captured!.location).toMatch(/canoe cove/i);
  });

  test("shows the saved station and its code", () => {
    render(
      <TidePanel
        value={{ ...baseConfig(), stationCode: "01710", location: "Canoe Cove" }}
        onChange={() => {}}
      />,
    );
    expect(screen.getByText(/canoe cove/i)).toBeTruthy();
    expect(screen.getByText(/01710/)).toBeTruthy();
  });

  test("enabled checkbox toggle propagates", () => {
    let captured: Tide | null = null;
    render(
      <TidePanel
        value={{ ...baseConfig(), enabled: false }}
        onChange={(t) => {
          captured = t;
        }}
      />,
    );
    fireEvent.click(screen.getByRole("checkbox"));
    expect(captured).toBeTruthy();
    expect(captured!.enabled).toBe(true);
  });
});

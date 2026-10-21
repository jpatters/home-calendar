import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import HockeyPanel from "./HockeyPanel";
import type { Hockey } from "../types";

function baseConfig(): Hockey {
  return { enabled: true, teamAbbr: "", teamName: "" };
}

const leafsResponse = [
  {
    abbrev: "TOR",
    name: "Toronto Maple Leafs",
    placeName: "Toronto",
    commonName: "Maple Leafs",
  },
];

beforeEach(() => {
  vi.stubGlobal(
    "fetch",
    vi.fn(() =>
      Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve(leafsResponse),
      } as unknown as Response),
    ),
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
  cleanup();
});

describe("HockeyPanel", () => {
  test("typing a team name, picking a result, surfaces abbrev/name via onChange", async () => {
    let captured: Hockey | null = null;
    render(
      <HockeyPanel
        value={baseConfig()}
        onChange={(h) => {
          captured = h;
        }}
      />,
    );

    fireEvent.change(screen.getByPlaceholderText(/team/i), {
      target: { value: "leaf" },
    });

    const button = await screen.findByRole("button", { name: /maple leafs/i });
    expect(fetch).toHaveBeenCalledWith(
      expect.stringContaining("/api/hockey/teams?q=leaf"),
      expect.anything(),
    );
    fireEvent.click(button);

    expect(captured).toBeTruthy();
    expect(captured!.teamAbbr).toBe("TOR");
    expect(captured!.teamName).toBe("Toronto Maple Leafs");
  });

  test("enabled checkbox toggle propagates", () => {
    let captured: Hockey | null = null;
    render(
      <HockeyPanel
        value={{ ...baseConfig(), enabled: false }}
        onChange={(h) => {
          captured = h;
        }}
      />,
    );
    fireEvent.click(screen.getByRole("checkbox"));
    expect(captured).toBeTruthy();
    expect(captured!.enabled).toBe(true);
  });

  test("shows the saved team", () => {
    render(
      <HockeyPanel
        value={{ enabled: true, teamAbbr: "TOR", teamName: "Toronto Maple Leafs" }}
        onChange={() => {}}
      />,
    );
    expect(screen.getByText(/Toronto Maple Leafs \(TOR\)/)).toBeTruthy();
  });
});

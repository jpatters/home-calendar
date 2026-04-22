import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import BaseballPanel from "./BaseballPanel";
import type { Baseball } from "../types";

function baseConfig(): Baseball {
  return { enabled: true, teamId: 0, teamName: "", teamAbbr: "" };
}

const yankeesResponse = [
  {
    id: 147,
    name: "New York Yankees",
    teamName: "Yankees",
    abbreviation: "NYY",
    locationName: "New York",
  },
];

beforeEach(() => {
  vi.stubGlobal(
    "fetch",
    vi.fn(() =>
      Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve(yankeesResponse),
      } as unknown as Response),
    ),
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
  cleanup();
});

describe("BaseballPanel", () => {
  test("typing a team name, picking a result, surfaces id/name/abbr via onChange", async () => {
    let captured: Baseball | null = null;
    render(
      <BaseballPanel
        value={baseConfig()}
        onChange={(b) => {
          captured = b;
        }}
      />,
    );

    fireEvent.change(screen.getByPlaceholderText(/team/i), {
      target: { value: "yank" },
    });

    const button = await screen.findByRole("button", { name: /yankees/i });
    fireEvent.click(button);

    expect(captured).toBeTruthy();
    expect(captured!.teamId).toBe(147);
    expect(captured!.teamAbbr).toBe("NYY");
    expect(captured!.teamName).toMatch(/yankees/i);
  });

  test("enabled checkbox toggle propagates", () => {
    let captured: Baseball | null = null;
    render(
      <BaseballPanel
        value={{ ...baseConfig(), enabled: false }}
        onChange={(b) => {
          captured = b;
        }}
      />,
    );
    const toggle = screen.getByRole("checkbox");
    fireEvent.click(toggle);
    expect(captured).toBeTruthy();
    expect(captured!.enabled).toBe(true);
  });
});

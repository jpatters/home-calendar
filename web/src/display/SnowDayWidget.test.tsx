import { afterEach, describe, expect, test } from "vitest";
import { cleanup, render } from "@testing-library/react";
import SnowDayWidget from "./SnowDayWidget";
import type { SnowDay, SnowDaySnapshot } from "../types";

afterEach(() => {
  cleanup();
});

function makeConfig(): SnowDay {
  return { enabled: true, url: "https://example.com" };
}

function makeSnowday(): SnowDaySnapshot {
  return {
    updatedAt: "2026-01-14T22:00:00Z",
    url: "https://example.com",
    location: "Test",
    regionName: "Test Region",
    morningTime: "2026-01-15T08:00:00Z",
    probability: 42,
    score: 0.42,
    category: "Likely",
  };
}

describe("SnowDayWidget icon", () => {
  test("renders an svg inside the snowday icon slot", () => {
    const { container } = render(
      <SnowDayWidget snowday={makeSnowday()} config={makeConfig()} />,
    );
    const iconEl = container.querySelector(".snowday-icon");
    expect(iconEl).not.toBeNull();
    expect(iconEl!.querySelector("svg")).not.toBeNull();
  });

  test("does not render the snowflake emoji character", () => {
    const { container } = render(
      <SnowDayWidget snowday={makeSnowday()} config={makeConfig()} />,
    );
    const text = container.textContent ?? "";
    expect(text).not.toContain("❄");
  });
});

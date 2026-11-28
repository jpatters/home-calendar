import { describe, expect, test } from "vitest";
import { sceneFor, type SceneInput } from "./scene";
import type { WeatherSnapshot } from "../../types";
import outfitFixture from "./outfits.fixture.json";

function at(y: number, mo: number, d: number, h = 12, mi = 0): Date {
  return new Date(y, mo - 1, d, h, mi);
}

function pad(n: number): string {
  return String(n).padStart(2, "0");
}

function localDate(d: Date): string {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

function weatherFor(
  now: Date,
  {
    code = 0,
    dailyCode = 0,
    sunrise = "07:00",
    sunset = "19:00",
  }: { code?: number; dailyCode?: number; sunrise?: string; sunset?: string } = {},
): WeatherSnapshot {
  const date = localDate(now);
  const yesterday = localDate(new Date(now.getFullYear(), now.getMonth(), now.getDate() - 1));
  return {
    updatedAt: now.toISOString(),
    units: "metric",
    timezone: "America/Halifax",
    current: {
      time: `${date}T12:00`,
      temperatureC: 10,
      apparentC: 9,
      humidity: 50,
      windSpeed: 5,
      weatherCode: code,
      isDay: true,
      precipitation: 0,
    },
    daily: [
      { date: yesterday, maxC: 10, minC: 0, weatherCode: 0, sunrise: `${yesterday}T05:00`, sunset: `${yesterday}T22:00`, precipMM: 0, windSpeedMax: 5 },
      { date, maxC: 10, minC: 0, weatherCode: dailyCode, sunrise: `${date}T${sunrise}`, sunset: `${date}T${sunset}`, precipMM: 0, windSpeedMax: 5 },
    ],
  };
}

function input(overrides: Partial<SceneInput> = {}): SceneInput {
  return {
    now: at(2026, 7, 15),
    weather: null,
    agentState: "idle",
    quietStart: "22:00",
    quietEnd: "07:00",
    ...overrides,
  };
}

// Shared with the server (internal/agent/outfit_test.go) so the outfit Ziggy
// describes always matches the one drawn here.
const outfitCases: { name: string; at: string; quietStart?: string; quietEnd?: string; outfit: string }[] =
  outfitFixture;

describe("sceneFor outfit", () => {
  test.each(outfitCases.map((c) => [c.name, c] as const))("%s", (_name, c) => {
    const [date, time] = c.at.split("T");
    const [y, mo, d] = date.split("-").map(Number);
    const [h, mi] = time.split(":").map(Number);
    const quiet = c.quietStart ? { quietStart: c.quietStart, quietEnd: c.quietEnd } : {};
    expect(sceneFor(input({ now: at(y, mo, d, h, mi), ...quiet })).outfit).toBe(c.outfit);
  });
});

describe("sceneFor pose", () => {
  test.each([
    ["idle at noon", "idle", at(2026, 7, 15, 12), "idle"],
    ["idle at 23:30 sleeps (quiet hours wrap midnight)", "idle", at(2026, 7, 15, 23, 30), "sleep"],
    ["idle at 03:00 sleeps", "idle", at(2026, 7, 15, 3), "sleep"],
    ["idle at 07:00 is awake (quiet end is exclusive)", "idle", at(2026, 7, 15, 7), "idle"],
    ["idle at 22:00 sleeps (quiet start is inclusive)", "idle", at(2026, 7, 15, 22), "sleep"],
    ["thinking at noon", "thinking", at(2026, 7, 15, 12), "think"],
    ["talking at noon", "talking", at(2026, 7, 15, 12), "talk"],
    ["error at noon", "error", at(2026, 7, 15, 12), "confused"],
    ["error during quiet hours", "error", at(2026, 7, 15, 23, 30), "confused"],
  ] as const)("%s", (_name, agentState, now, pose) => {
    expect(sceneFor(input({ agentState, now })).pose).toBe(pose);
  });

  test.each([
    ["checking the weather looks out the window", "get_weather", at(2026, 7, 15, 12), "lookout"],
    ["checking the weather at night still looks out the window", "get_weather", at(2026, 7, 15, 23, 30), "lookout"],
    ["running another tool keeps thinking", "get_scores", at(2026, 7, 15, 12), "think"],
  ] as const)("%s", (_name, tool, now, pose) => {
    expect(sceneFor(input({ agentState: "thinking", tool, now })).pose).toBe(pose);
  });

  test("waiting for an answer keeps the assistant up during quiet hours", () => {
    expect(sceneFor(input({ agentState: "idle", awaitingReply: true, now: at(2026, 7, 15, 23, 30) })).pose).toBe("listen");
  });

  test("waiting for an answer gives way to a new turn", () => {
    expect(sceneFor(input({ agentState: "thinking", awaitingReply: true })).pose).toBe("think");
  });

  test("a weather tool reported with a finished turn does not look out the window", () => {
    expect(sceneFor(input({ agentState: "talking", tool: "get_weather" })).pose).toBe("talk");
  });

  test("quiet hours that do not wrap midnight", () => {
    const base = { quietStart: "13:00", quietEnd: "15:00" };
    expect(sceneFor(input({ ...base, now: at(2026, 7, 15, 14) })).pose).toBe("sleep");
    expect(sceneFor(input({ ...base, now: at(2026, 7, 15, 12) })).pose).toBe("idle");
    expect(sceneFor(input({ ...base, now: at(2026, 7, 15, 23, 30) })).pose).toBe("idle");
  });
});

describe("sceneFor Biscuit", () => {
  test.each([
    ["runs on her wheel all night", at(2026, 7, 15, 23, 30), "wheel"],
    ["still running at 3am", at(2026, 7, 15, 3, 7), "wheel"],
    ["naps in her nest by day", at(2026, 7, 15, 12, 5), "nap"],
    ["takes a short run at the top of the hour", at(2026, 7, 15, 12, 0), "wheel"],
    ["still on that run a minute later", at(2026, 7, 15, 12, 1), "wheel"],
    ["back in the nest after two minutes", at(2026, 7, 15, 12, 2), "nap"],
    ["runs again each quarter hour", at(2026, 7, 15, 14, 45), "wheel"],
  ] as const)("%s", (_name, now, biscuit) => {
    expect(sceneFor(input({ now })).biscuit).toBe(biscuit);
  });
});

describe("sceneFor sky", () => {
  test.each([
    ["well before sunrise", 5, 0, "night"],
    ["30 min before sunrise", 6, 30, "dusk"],
    ["31 min after sunrise", 7, 31, "day"],
    ["midday", 12, 0, "day"],
    ["shortly after sunset", 19, 25, "dusk"],
    ["an hour after sunset", 20, 0, "night"],
  ])("%s", (_name, h, mi, sky) => {
    const now = at(2026, 6, 10, h as number, mi as number);
    expect(sceneFor(input({ now, weather: weatherFor(now) })).sky).toBe(sky);
  });

  test("uses today's sunrise and sunset, not another day's", () => {
    const now = at(2026, 6, 10, 21, 0);
    expect(sceneFor(input({ now, weather: weatherFor(now, { sunset: "19:00" }) })).sky).toBe("night");
  });

  test.each([
    [6, 59, "night"],
    [7, 0, "day"],
    [18, 59, "day"],
    [19, 0, "night"],
  ])("without weather, %i:%i (h:m) is %s", (h, mi, sky) => {
    expect(sceneFor(input({ now: at(2026, 6, 10, h as number, mi as number), weather: null })).sky).toBe(sky);
  });
});

describe("sceneFor sky fallback", () => {
  test.each([
    [12, 0, "day"],
    [20, 0, "night"],
  ])("weather without an entry for today falls back to the clock: %i:%i is %s", (h, mi, sky) => {
    const now = at(2026, 6, 10, h as number, mi as number);
    const weather = weatherFor(now);
    weather.daily = weather.daily.filter((d) => d.date !== "2026-06-10");
    expect(sceneFor(input({ now, weather })).sky).toBe(sky);
  });
});

describe("sceneFor conditions", () => {
  test.each([
    [0, "none"],
    [1, "none"],
    [2, "partly"],
    [3, "overcast"],
    [4, "none"],
    [44, "none"],
    [45, "fog"],
    [48, "fog"],
    [49, "none"],
    [50, "none"],
    [51, "rain"],
    [56, "rain"],
    [57, "rain"],
    [66, "rain"],
    [67, "rain"],
    [68, "none"],
    [70, "none"],
    [71, "snow"],
    [77, "snow"],
    [78, "none"],
    [79, "none"],
    [80, "rain"],
    [82, "rain"],
    [83, "none"],
    [84, "none"],
    [85, "snow"],
    [86, "snow"],
    [87, "none"],
    [94, "none"],
    [95, "thunder"],
    [99, "thunder"],
  ])("current WMO code %i is %s, regardless of today's forecast code", (code, conditions) => {
    const now = at(2026, 6, 10);
    const dailyCode = conditions === "snow" ? 61 : 73;
    expect(sceneFor(input({ now, weather: weatherFor(now, { code, dailyCode }) })).conditions).toBe(conditions);
  });

  test("no weather means a clear window", () => {
    expect(sceneFor(input({ weather: null })).conditions).toBe("none");
  });
});

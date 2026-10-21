import { afterEach, describe, expect, test } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import HockeyWidget from "./HockeyWidget";
import type { Hockey, HockeyGame, HockeySnapshot } from "../types";

afterEach(() => cleanup());

const config: Hockey = {
  enabled: true,
  teamAbbr: "TOR",
  teamName: "Toronto Maple Leafs",
};

function snapshot(partial: Partial<HockeySnapshot>): HockeySnapshot {
  return {
    updatedAt: "2026-10-21T12:00:00Z",
    teamAbbr: "TOR",
    teamName: "Toronto Maple Leafs",
    liveGame: null,
    latestGame: null,
    nextGame: null,
    ...partial,
  };
}

function finalGame(partial: Partial<HockeyGame> = {}): HockeyGame {
  return {
    gameTime: "2026-10-20T23:00:00Z",
    opponent: "Columbus Blue Jackets",
    opponentAbbr: "CBJ",
    homeAway: "away",
    venue: "Nationwide Arena",
    status: "OFF",
    isFinal: true,
    isLive: false,
    teamScore: 6,
    opponentScore: 2,
    gameType: 2,
    period: 3,
    periodType: "REG",
    ...partial,
  };
}

function nextGame(partial: Partial<HockeyGame> = {}): HockeyGame {
  return {
    gameTime: "2030-10-22T23:00:00Z",
    opponent: "New York Rangers",
    opponentAbbr: "NYR",
    homeAway: "home",
    venue: "Scotiabank Arena",
    status: "FUT",
    isFinal: false,
    isLive: false,
    teamScore: 0,
    opponentScore: 0,
    gameType: 2,
    ...partial,
  };
}

function liveGame(partial: Partial<HockeyGame> = {}): HockeyGame {
  return {
    gameTime: "2026-10-22T23:00:00Z",
    opponent: "New York Rangers",
    opponentAbbr: "NYR",
    homeAway: "home",
    venue: "Scotiabank Arena",
    status: "LIVE",
    isFinal: false,
    isLive: true,
    teamScore: 3,
    opponentScore: 1,
    gameType: 2,
    period: 2,
    periodType: "REG",
    clock: "12:34",
    inIntermission: false,
    ...partial,
  };
}

describe("HockeyWidget", () => {
  test("shows unavailable copy when snapshot is null", () => {
    render(<HockeyWidget hockey={null} config={config} />);
    expect(screen.getByText(/hockey unavailable/i)).toBeTruthy();
  });

  test("shows configure prompt when no team is set", () => {
    render(
      <HockeyWidget
        hockey={snapshot({})}
        config={{ ...config, teamAbbr: "", teamName: "" }}
      />,
    );
    expect(screen.getByText(/pick a team/i)).toBeTruthy();
  });

  test("renders latest score showing both teams, scores and Final", () => {
    render(<HockeyWidget hockey={snapshot({ latestGame: finalGame() })} config={config} />);
    expect(screen.getByText("TOR")).toBeTruthy();
    expect(screen.getByText("CBJ")).toBeTruthy();
    expect(screen.getByText("6")).toBeTruthy();
    expect(screen.getByText("2")).toBeTruthy();
    expect(screen.getByText(/^final$/i)).toBeTruthy();
  });

  test("labels an overtime final as Final/OT", () => {
    render(
      <HockeyWidget
        hockey={snapshot({ latestGame: finalGame({ period: 4, periodType: "OT", teamScore: 3, opponentScore: 2 }) })}
        config={config}
      />,
    );
    expect(screen.getByText(/final\s*\/\s*OT/i)).toBeTruthy();
  });

  test("renders next game opponent and venue", () => {
    render(<HockeyWidget hockey={snapshot({ nextGame: nextGame() })} config={config} />);
    expect(screen.getByText("New York Rangers")).toBeTruthy();
    expect(screen.getByText(/scotiabank arena/i)).toBeTruthy();
  });

  test("per-section empty states", () => {
    render(<HockeyWidget hockey={snapshot({})} config={config} />);
    expect(screen.getByText(/no recent game/i)).toBeTruthy();
    expect(screen.getByText(/no upcoming game/i)).toBeTruthy();
  });

  test("renders live game with LIVE pill, score, period and clock", () => {
    render(<HockeyWidget hockey={snapshot({ liveGame: liveGame() })} config={config} />);
    expect(screen.getByText(/live/i)).toBeTruthy();
    expect(screen.getByText("TOR")).toBeTruthy();
    expect(screen.getByText("NYR")).toBeTruthy();
    expect(screen.getByText("3")).toBeTruthy();
    expect(screen.getByText("1")).toBeTruthy();
    expect(screen.getByText(/2nd/)).toBeTruthy();
    expect(screen.getByText("12:34")).toBeTruthy();
  });

  test("shows Intermission instead of a clock between periods", () => {
    render(
      <HockeyWidget
        hockey={snapshot({ liveGame: liveGame({ period: 1, clock: "00:00", inIntermission: true }) })}
        config={config}
      />,
    );
    expect(screen.getByText(/intermission/i)).toBeTruthy();
    expect(screen.queryByText("00:00")).toBeNull();
  });

  test("shows SO without a clock during a shootout", () => {
    render(
      <HockeyWidget
        hockey={snapshot({ liveGame: liveGame({ period: 5, periodType: "SO", clock: "00:00" }) })}
        config={config}
      />,
    );
    expect(screen.getByText(/^SO$/)).toBeTruthy();
    expect(screen.queryByText("00:00")).toBeNull();
  });

  test("hides Latest section when a live game is in progress", () => {
    render(
      <HockeyWidget
        hockey={snapshot({ liveGame: liveGame(), latestGame: finalGame() })}
        config={config}
      />,
    );
    expect(screen.queryByText(/columbus/i)).toBeNull();
    expect(screen.queryByText("CBJ")).toBeNull();
  });
});

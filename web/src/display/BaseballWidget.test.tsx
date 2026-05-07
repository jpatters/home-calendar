import { afterEach, describe, expect, test } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import BaseballWidget from "./BaseballWidget";
import type { Baseball, BaseballSnapshot } from "../types";

afterEach(() => cleanup());

const config: Baseball = {
  enabled: true,
  teamId: 147,
  teamName: "New York Yankees",
  teamAbbr: "NYY",
};

function snapshot(partial: Partial<BaseballSnapshot>): BaseballSnapshot {
  return {
    updatedAt: "2026-04-21T12:00:00Z",
    teamId: 147,
    teamName: "New York Yankees",
    teamAbbr: "NYY",
    liveGame: null,
    latestGame: null,
    nextGame: null,
    ...partial,
  };
}

describe("BaseballWidget", () => {
  test("shows unavailable copy when snapshot is null", () => {
    render(<BaseballWidget baseball={null} config={config} />);
    expect(screen.getByText(/baseball unavailable/i)).toBeTruthy();
  });

  test("shows configure prompt when no team is set", () => {
    render(
      <BaseballWidget
        baseball={snapshot({})}
        config={{ ...config, teamId: 0, teamName: "", teamAbbr: "" }}
      />,
    );
    expect(screen.getByText(/pick a team/i)).toBeTruthy();
  });

  test("renders latest score showing both teams and scores", () => {
    render(
      <BaseballWidget
        baseball={snapshot({
          latestGame: {
            gameTime: "2026-04-20T23:05:00Z",
            opponent: "Boston Red Sox",
            opponentAbbr: "BOS",
            homeAway: "home",
            venue: "Yankee Stadium",
            status: "Final",
            isFinal: true,
            teamScore: 5,
            opponentScore: 3,
            gameType: "R",
          },
        })}
        config={config}
      />,
    );
    expect(screen.getByText(/NYY/)).toBeTruthy();
    expect(screen.getByText(/BOS/)).toBeTruthy();
    expect(screen.getByText("5")).toBeTruthy();
    expect(screen.getByText("3")).toBeTruthy();
    expect(screen.getByText(/final/i)).toBeTruthy();
  });

  test("renders next game opponent and venue", () => {
    render(
      <BaseballWidget
        baseball={snapshot({
          nextGame: {
            gameTime: "2030-06-14T23:05:00Z",
            opponent: "New York Mets",
            opponentAbbr: "NYM",
            homeAway: "away",
            venue: "Citi Field",
            status: "Scheduled",
            isFinal: false,
            teamScore: 0,
            opponentScore: 0,
            gameType: "R",
          },
        })}
        config={config}
      />,
    );
    expect(screen.getByText("New York Mets")).toBeTruthy();
    expect(screen.getByText(/citi field/i)).toBeTruthy();
  });

  test("per-half empty states: next missing shows 'No upcoming game'", () => {
    render(
      <BaseballWidget
        baseball={snapshot({
          latestGame: {
            gameTime: "2026-04-20T23:05:00Z",
            opponent: "Boston Red Sox",
            opponentAbbr: "BOS",
            homeAway: "home",
            venue: "Yankee Stadium",
            status: "Final",
            isFinal: true,
            teamScore: 5,
            opponentScore: 3,
            gameType: "R",
          },
          nextGame: null,
        })}
        config={config}
      />,
    );
    expect(screen.getByText(/no upcoming game/i)).toBeTruthy();
    // latest section still renders Final state + opponent abbr.
    expect(screen.getByText(/final/i)).toBeTruthy();
    expect(screen.getByText("BOS")).toBeTruthy();
  });

  test("per-half empty states: latest missing shows 'No recent game'", () => {
    render(
      <BaseballWidget
        baseball={snapshot({
          latestGame: null,
          nextGame: {
            gameTime: "2030-06-14T23:05:00Z",
            opponent: "New York Mets",
            opponentAbbr: "NYM",
            homeAway: "away",
            venue: "Citi Field",
            status: "Scheduled",
            isFinal: false,
            teamScore: 0,
            opponentScore: 0,
            gameType: "R",
          },
        })}
        config={config}
      />,
    );
    expect(screen.getByText(/no recent game/i)).toBeTruthy();
  });

  test("renders live game with LIVE pill, score, inning, and outs", () => {
    render(
      <BaseballWidget
        baseball={snapshot({
          liveGame: {
            gameTime: "2026-04-22T23:00:00Z",
            opponent: "Boston Red Sox",
            opponentAbbr: "BOS",
            homeAway: "home",
            venue: "Yankee Stadium",
            status: "In Progress",
            isFinal: false,
            isLive: true,
            teamScore: 3,
            opponentScore: 1,
            gameType: "R",
            inning: 5,
            inningHalf: "top",
            outs: 2,
          },
        })}
        config={config}
      />,
    );
    expect(screen.getByText(/live/i)).toBeTruthy();
    // Both teams visible.
    expect(screen.getByText("NYY")).toBeTruthy();
    expect(screen.getByText("BOS")).toBeTruthy();
    // Both scores visible.
    expect(screen.getByText("3")).toBeTruthy();
    expect(screen.getByText("1")).toBeTruthy();
    // Inning + half displayed (e.g. "Top 5th").
    expect(screen.getByText(/top.*5th/i)).toBeTruthy();
    // Outs displayed.
    expect(screen.getByText(/2 out/i)).toBeTruthy();
  });

  test("hides Latest section when a live game is in progress", () => {
    render(
      <BaseballWidget
        baseball={snapshot({
          liveGame: {
            gameTime: "2026-04-22T23:00:00Z",
            opponent: "Boston Red Sox",
            opponentAbbr: "BOS",
            homeAway: "home",
            status: "In Progress",
            isFinal: false,
            isLive: true,
            teamScore: 1,
            opponentScore: 0,
            gameType: "R",
            inning: 2,
            inningHalf: "bottom",
            outs: 0,
          },
          latestGame: {
            gameTime: "2026-04-20T23:05:00Z",
            opponent: "Tampa Bay Rays",
            opponentAbbr: "TB",
            homeAway: "away",
            status: "Final",
            isFinal: true,
            isLive: false,
            teamScore: 7,
            opponentScore: 4,
            gameType: "R",
          },
        })}
        config={config}
      />,
    );
    // Latest opponent must NOT be rendered while live game is showing.
    expect(screen.queryByText(/tampa bay/i)).toBeNull();
    expect(screen.queryByText("TB")).toBeNull();
  });

});

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
});

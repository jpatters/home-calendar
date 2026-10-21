import type { Hockey, HockeyGame, HockeySnapshot } from "../types";
import { formatGameTime } from "./gameFormat";
import { formatFinal, formatPeriod } from "./hockeyFormat";

interface Props {
  hockey: HockeySnapshot | null;
  config: Hockey | undefined;
}

export default function HockeyWidget({ hockey, config }: Props) {
  if (!hockey) {
    return (
      <div className="widget hockey-widget hockey-widget-empty">
        <div className="scores-empty">Hockey unavailable</div>
      </div>
    );
  }
  if (!config || config.teamAbbr === "") {
    return (
      <div className="widget hockey-widget hockey-widget-empty">
        <div className="scores-empty">Pick a team in admin</div>
      </div>
    );
  }
  const teamAbbr = hockey.teamAbbr || config.teamAbbr;
  const liveGame = hockey.liveGame;
  return (
    <div className="widget hockey-widget">
      {liveGame ? (
        <div className="scores-section">
          <div className="scores-section-label scores-live-label">
            <span className="scores-live-dot" />
            LIVE
          </div>
          <LiveGameRow teamAbbr={teamAbbr} game={liveGame} />
        </div>
      ) : (
        <div className="scores-section">
          <div className="scores-section-label">Latest</div>
          {hockey.latestGame ? (
            <LatestGameRow teamAbbr={teamAbbr} game={hockey.latestGame} />
          ) : (
            <div className="scores-empty">No recent game</div>
          )}
        </div>
      )}
      <div className="scores-section">
        <div className="scores-section-label">Next</div>
        {hockey.nextGame ? (
          <NextGameRow teamAbbr={teamAbbr} game={hockey.nextGame} />
        ) : (
          <div className="scores-empty">No upcoming game</div>
        )}
      </div>
    </div>
  );
}

function venueLine(game: HockeyGame): string {
  return game.homeAway === "home" ? `vs ${game.opponent}` : `@ ${game.venue || game.opponent}`;
}

function LiveGameRow({ teamAbbr, game }: { teamAbbr: string; game: HockeyGame }) {
  const periodLabel = formatPeriod(game.period, game.periodType);
  // A shootout has no running clock; the feed reports 00:00 throughout.
  const showClock = Boolean(game.clock) && game.periodType !== "SO";
  return (
    <div className="scores-live">
      <div className="scores-scoreline">
        <span className="scores-team">{teamAbbr}</span>
        <span className="scores-score">{game.teamScore}</span>
        <span className="scores-scoreline-sep">·</span>
        <span className="scores-team">{game.opponentAbbr || game.opponent}</span>
        <span className="scores-score">{game.opponentScore}</span>
      </div>
      <div className="scores-meta">
        {periodLabel && <span className="scores-period">{periodLabel}</span>}
        {game.inIntermission ? (
          <span className="scores-clock">Intermission</span>
        ) : (
          showClock && <span className="scores-clock">{game.clock}</span>
        )}
        <span className="scores-venue">{venueLine(game)}</span>
      </div>
    </div>
  );
}

function LatestGameRow({ teamAbbr, game }: { teamAbbr: string; game: HockeyGame }) {
  const teamWon = game.teamScore > game.opponentScore;
  return (
    <div className="scores-final">
      <div className="scores-scoreline">
        <span className={teamWon ? "scores-team scores-winner" : "scores-team"}>{teamAbbr}</span>
        <span className="scores-score">{game.teamScore}</span>
        <span className="scores-scoreline-sep">·</span>
        <span className={!teamWon ? "scores-team scores-winner" : "scores-team"}>
          {game.opponentAbbr || game.opponent}
        </span>
        <span className="scores-score">{game.opponentScore}</span>
      </div>
      <div className="scores-meta">
        <span className="scores-status">{formatFinal(game.periodType)}</span>
        <span className="scores-venue">{venueLine(game)}</span>
      </div>
    </div>
  );
}

function NextGameRow({ teamAbbr, game }: { teamAbbr: string; game: HockeyGame }) {
  const prefix = game.homeAway === "home" ? "vs" : "@";
  return (
    <div className="scores-upcoming">
      <div className="scores-matchup">
        <span className="scores-team">{teamAbbr}</span>
        <span className="scores-vs">{prefix}</span>
        <span className="scores-team">{game.opponent}</span>
      </div>
      <div className="scores-meta">
        <span className="scores-time">{formatGameTime(game.gameTime)}</span>
        {game.venue && <span className="scores-venue">{game.venue}</span>}
      </div>
    </div>
  );
}

import type { Baseball, BaseballGame, BaseballSnapshot } from "../types";
import { formatInning } from "./baseballFormat";
import { formatGameTime } from "./gameFormat";

interface Props {
  baseball: BaseballSnapshot | null;
  config: Baseball | undefined;
}

export default function BaseballWidget({ baseball, config }: Props) {
  if (!baseball) {
    return (
      <div className="widget baseball-widget baseball-widget-empty">
        <div className="scores-empty">Baseball unavailable</div>
      </div>
    );
  }
  if (!config || config.teamId === 0) {
    return (
      <div className="widget baseball-widget baseball-widget-empty">
        <div className="scores-empty">Pick a team in admin</div>
      </div>
    );
  }
  const teamAbbr = baseball.teamAbbr || config.teamAbbr || "Team";
  const liveGame = baseball.liveGame;
  return (
    <div className="widget baseball-widget">
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
          {baseball.latestGame ? (
            <LatestGameRow teamAbbr={teamAbbr} game={baseball.latestGame} />
          ) : (
            <div className="scores-empty">No recent game</div>
          )}
        </div>
      )}
      <div className="scores-section">
        <div className="scores-section-label">Next</div>
        {baseball.nextGame ? (
          <NextGameRow teamAbbr={teamAbbr} game={baseball.nextGame} />
        ) : (
          <div className="scores-empty">No upcoming game</div>
        )}
      </div>
    </div>
  );
}

function LiveGameRow({ teamAbbr, game }: { teamAbbr: string; game: BaseballGame }) {
  const inningLabel = formatInning(game.inningHalf, game.inning);
  const showOuts = game.inningHalf === "top" || game.inningHalf === "bottom";
  const venueLine = game.homeAway === "home" ? `vs ${game.opponent}` : `@ ${game.venue || game.opponent}`;
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
        {inningLabel && <span className="scores-inning">{inningLabel}</span>}
        {showOuts && <span className="scores-outs">{game.outs ?? 0} out</span>}
        <span className="scores-venue">{venueLine}</span>
      </div>
    </div>
  );
}

function LatestGameRow({ teamAbbr, game }: { teamAbbr: string; game: BaseballGame }) {
  const teamWon = game.teamScore > game.opponentScore;
  const venueLine = game.homeAway === "home" ? `vs ${game.opponent}` : `@ ${game.venue || game.opponent}`;
  return (
    <div className="scores-final">
      <div className="scores-scoreline">
        <span className={teamWon ? "scores-team scores-winner" : "scores-team"}>
          {teamAbbr}
        </span>
        <span className="scores-score">{game.teamScore}</span>
        <span className="scores-scoreline-sep">·</span>
        <span className={!teamWon ? "scores-team scores-winner" : "scores-team"}>
          {game.opponentAbbr || game.opponent}
        </span>
        <span className="scores-score">{game.opponentScore}</span>
      </div>
      <div className="scores-meta">
        <span className="scores-status">Final</span>
        <span className="scores-venue">{venueLine}</span>
      </div>
    </div>
  );
}

function NextGameRow({ teamAbbr, game }: { teamAbbr: string; game: BaseballGame }) {
  const prefix = game.homeAway === "home" ? "vs" : "@";
  const opponentLabel = game.opponent;
  return (
    <div className="scores-upcoming">
      <div className="scores-matchup">
        <span className="scores-team">{teamAbbr}</span>
        <span className="scores-vs">{prefix}</span>
        <span className="scores-team">{opponentLabel}</span>
      </div>
      <div className="scores-meta">
        <span className="scores-time">{formatGameTime(game.gameTime)}</span>
        {game.venue && <span className="scores-venue">{game.venue}</span>}
      </div>
    </div>
  );
}

import type { Baseball, BaseballGame, BaseballSnapshot } from "../types";
import { formatGameTime } from "./baseballFormat";

interface Props {
  baseball: BaseballSnapshot | null;
  config: Baseball | undefined;
}

export default function BaseballWidget({ baseball, config }: Props) {
  if (!baseball) {
    return (
      <div className="widget baseball-widget baseball-widget-empty">
        <div className="baseball-empty">Baseball unavailable</div>
      </div>
    );
  }
  if (!config || config.teamId === 0) {
    return (
      <div className="widget baseball-widget baseball-widget-empty">
        <div className="baseball-empty">Pick a team in admin</div>
      </div>
    );
  }
  const teamAbbr = baseball.teamAbbr || config.teamAbbr || "Team";
  return (
    <div className="widget baseball-widget">
      <div className="baseball-section">
        <div className="baseball-section-label">Latest</div>
        {baseball.latestGame ? (
          <LatestGameRow teamAbbr={teamAbbr} game={baseball.latestGame} />
        ) : (
          <div className="baseball-empty">No recent game</div>
        )}
      </div>
      <div className="baseball-section">
        <div className="baseball-section-label">Next</div>
        {baseball.nextGame ? (
          <NextGameRow teamAbbr={teamAbbr} game={baseball.nextGame} />
        ) : (
          <div className="baseball-empty">No upcoming game</div>
        )}
      </div>
    </div>
  );
}

function LatestGameRow({ teamAbbr, game }: { teamAbbr: string; game: BaseballGame }) {
  const teamWon = game.teamScore > game.opponentScore;
  const venueLine = game.homeAway === "home" ? `vs ${game.opponent}` : `@ ${game.venue || game.opponent}`;
  return (
    <div className="baseball-final">
      <div className="baseball-scoreline">
        <span className={teamWon ? "baseball-team baseball-winner" : "baseball-team"}>
          {teamAbbr}
        </span>
        <span className="baseball-score">{game.teamScore}</span>
        <span className="baseball-scoreline-sep">·</span>
        <span className={!teamWon ? "baseball-team baseball-winner" : "baseball-team"}>
          {game.opponentAbbr || game.opponent}
        </span>
        <span className="baseball-score">{game.opponentScore}</span>
      </div>
      <div className="baseball-meta">
        <span className="baseball-status">Final</span>
        <span className="baseball-venue">{venueLine}</span>
      </div>
    </div>
  );
}

function NextGameRow({ teamAbbr, game }: { teamAbbr: string; game: BaseballGame }) {
  const prefix = game.homeAway === "home" ? "vs" : "@";
  const opponentLabel = game.opponent;
  return (
    <div className="baseball-upcoming">
      <div className="baseball-matchup">
        <span className="baseball-team">{teamAbbr}</span>
        <span className="baseball-vs">{prefix}</span>
        <span className="baseball-team">{opponentLabel}</span>
      </div>
      <div className="baseball-meta">
        <span className="baseball-time">{formatGameTime(game.gameTime)}</span>
        {game.venue && <span className="baseball-venue">{game.venue}</span>}
      </div>
    </div>
  );
}

package baseball

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/jpatters/home-calendar/internal/types"
)

const (
	DefaultScheduleURL = "https://statsapi.mlb.com/api/v1/schedule"
	DefaultTeamsURL    = "https://statsapi.mlb.com/api/v1/teams"
	userAgent          = "home-calendar/1.0"
)

// allowedGameTypes are the MLB gameType codes we surface in the widget.
// R = regular season. F/D/L/W = wild-card / division / league / World Series.
// Excludes S (spring training), E (exhibition), A (all-star), I (intrasquad), P (preseason non-exhibition).
var allowedGameTypes = map[string]struct{}{
	"R": {}, "F": {}, "D": {}, "L": {}, "W": {},
}

type scheduleResponse struct {
	Dates []scheduleDate `json:"dates"`
}

type scheduleDate struct {
	Date  string         `json:"date"`
	Games []scheduleGame `json:"games"`
}

type scheduleGame struct {
	GamePk    int64             `json:"gamePk"`
	GameDate  string            `json:"gameDate"`
	GameType  string            `json:"gameType"`
	Status    scheduleStatus    `json:"status"`
	Teams     scheduleTeams     `json:"teams"`
	Venue     scheduleVenue     `json:"venue"`
	Linescore scheduleLinescore `json:"linescore"`
}

type scheduleStatus struct {
	AbstractGameState string `json:"abstractGameState"`
	DetailedState     string `json:"detailedState"`
}

type scheduleLinescore struct {
	CurrentInning int    `json:"currentInning"`
	InningHalf    string `json:"inningHalf"`
	InningState   string `json:"inningState"`
	Outs          int    `json:"outs"`
}

type scheduleTeams struct {
	Home scheduleSide `json:"home"`
	Away scheduleSide `json:"away"`
}

type scheduleSide struct {
	Team  scheduleTeam `json:"team"`
	Score int          `json:"score"`
}

type scheduleTeam struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Abbreviation string `json:"abbreviation"`
}

type scheduleVenue struct {
	Name string `json:"name"`
}

// Search fetches recent and upcoming games for the configured team from an
// MLB Stats-API-compatible endpoint and returns a snapshot with the latest
// completed game and the earliest upcoming game (regular season + playoffs
// only; spring training and exhibition games are filtered out).
func Search(ctx context.Context, client *http.Client, baseURL string, b types.Baseball, now time.Time) (*types.BaseballSnapshot, error) {
	if client == nil {
		client = http.DefaultClient
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("baseball: parse URL: %w", err)
	}
	q := u.Query()
	q.Set("sportId", "1")
	q.Set("teamId", strconv.Itoa(b.TeamID))
	q.Set("startDate", now.AddDate(0, 0, -7).Format("2006-01-02"))
	q.Set("endDate", now.AddDate(0, 0, 10).Format("2006-01-02"))
	q.Set("hydrate", "team,linescore,venue")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("baseball: MLB api http %d", resp.StatusCode)
	}

	var body scheduleResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("baseball: decode response: %w", err)
	}

	snap := &types.BaseballSnapshot{
		UpdatedAt: time.Now().UTC(),
		TeamID:    b.TeamID,
		TeamName:  b.TeamName,
		TeamAbbr:  b.TeamAbbr,
	}
	live, latest, next := pickGames(body, b.TeamID, now)
	snap.LiveGame = live
	snap.LatestGame = latest
	snap.NextGame = next
	return snap, nil
}

func pickGames(body scheduleResponse, teamID int, now time.Time) (*types.BaseballGame, *types.BaseballGame, *types.BaseballGame) {
	var live, latest, next *types.BaseballGame
	var liveTime, latestTime, nextTime time.Time

	for _, d := range body.Dates {
		for _, g := range d.Games {
			if _, ok := allowedGameTypes[g.GameType]; !ok {
				continue
			}
			gameTime, err := time.Parse(time.RFC3339, g.GameDate)
			if err != nil {
				continue
			}
			converted := convertGame(g, gameTime, teamID)
			if converted == nil {
				continue
			}
			switch {
			case isLive(g.Status):
				if live == nil || gameTime.After(liveTime) {
					live = converted
					liveTime = gameTime
				}
			case isCompleted(g.Status):
				if latest == nil || gameTime.After(latestTime) {
					latest = converted
					latestTime = gameTime
				}
			case isUpcoming(g.Status, gameTime, now):
				if next == nil || gameTime.Before(nextTime) {
					next = converted
					nextTime = gameTime
				}
			}
		}
	}
	return live, latest, next
}

func isCompleted(s scheduleStatus) bool {
	if s.AbstractGameState != "Final" {
		return false
	}
	switch s.DetailedState {
	case "Postponed", "Cancelled", "Suspended":
		return false
	}
	return true
}

func isLive(s scheduleStatus) bool {
	return s.AbstractGameState == "Live"
}

func isUpcoming(s scheduleStatus, gameTime, now time.Time) bool {
	if gameTime.Before(now) {
		return false
	}
	return s.AbstractGameState == "Preview"
}

func convertGame(g scheduleGame, gameTime time.Time, teamID int) *types.BaseballGame {
	var us, them scheduleSide
	var homeAway string
	switch {
	case g.Teams.Home.Team.ID == teamID:
		us = g.Teams.Home
		them = g.Teams.Away
		homeAway = "home"
	case g.Teams.Away.Team.ID == teamID:
		us = g.Teams.Away
		them = g.Teams.Home
		homeAway = "away"
	default:
		return nil
	}
	return &types.BaseballGame{
		GameTime:      gameTime,
		Opponent:      them.Team.Name,
		OpponentAbbr:  them.Team.Abbreviation,
		HomeAway:      homeAway,
		Venue:         g.Venue.Name,
		Status:        g.Status.DetailedState,
		IsFinal:       isCompleted(g.Status),
		IsLive:        isLive(g.Status),
		TeamScore:     us.Score,
		OpponentScore: them.Score,
		GameType:      g.GameType,
		Inning:        g.Linescore.CurrentInning,
		InningHalf:    normalizeInningHalf(g.Linescore.InningHalf, g.Linescore.InningState),
		Outs:          g.Linescore.Outs,
	}
}

// normalizeInningHalf returns lowercase "top"/"bottom"/"middle"/"end" — MLB
// returns title-case and sometimes only inningState for between-half states.
func normalizeInningHalf(half, state string) string {
	switch {
	case state == "Middle":
		return "middle"
	case state == "End":
		return "end"
	case half == "Top":
		return "top"
	case half == "Bottom":
		return "bottom"
	}
	return ""
}

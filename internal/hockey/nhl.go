package hockey

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jpatters/home-calendar/internal/types"
)

const (
	DefaultBaseURL = "https://api-web.nhle.com"
	userAgent      = "home-calendar/1.0"
)

// NHL gameType codes. 1 = preseason (never shown), 2 = regular season,
// 3 = playoffs.
const (
	gameTypeRegular  = 2
	gameTypePlayoffs = 3
)

// localized is the NHL API's {"default": "...", "fr": "..."} string wrapper.
type localized struct {
	Default string `json:"default"`
}

type scoreboardResponse struct {
	GamesByDate []scoreboardDate `json:"gamesByDate"`
}

type scoreboardDate struct {
	Date  string           `json:"date"`
	Games []scoreboardGame `json:"games"`
}

type scoreboardGame struct {
	ID                int64            `json:"id"`
	GameType          int              `json:"gameType"`
	StartTimeUTC      string           `json:"startTimeUTC"`
	GameState         string           `json:"gameState"`
	GameScheduleState string           `json:"gameScheduleState"`
	Venue             localized        `json:"venue"`
	AwayTeam          scoreboardTeam   `json:"awayTeam"`
	HomeTeam          scoreboardTeam   `json:"homeTeam"`
	PeriodDescriptor  periodDescriptor `json:"periodDescriptor"`
}

type scoreboardTeam struct {
	Abbrev string    `json:"abbrev"`
	Name   localized `json:"name"`
	Score  int       `json:"score"`
}

type periodDescriptor struct {
	Number     int    `json:"number"`
	PeriodType string `json:"periodType"`
}

// landingResponse is the subset of /v1/gamecenter/{id}/landing we read for
// a game in progress: the scoreboard itself carries no clock.
type landingResponse struct {
	Clock struct {
		TimeRemaining  string `json:"timeRemaining"`
		InIntermission bool   `json:"inIntermission"`
	} `json:"clock"`
	PeriodDescriptor periodDescriptor `json:"periodDescriptor"`
}

// Search fetches the configured team's scoreboard (recent and upcoming
// games) from an NHL-web-API-compatible host and returns a snapshot with the
// live game (if any), the latest completed game, and the next upcoming game.
// Preseason games and postponed/cancelled games are ignored. While a game is
// live the gamecenter landing is also read for the period clock; if that
// call fails the live game is still reported, just without a clock.
func Search(ctx context.Context, client *http.Client, baseURL string, h types.Hockey, now time.Time) (*types.HockeySnapshot, error) {
	if client == nil {
		client = http.DefaultClient
	}
	base := strings.TrimRight(baseURL, "/")
	abbr := h.TeamAbbr

	var board scoreboardResponse
	if err := getJSON(ctx, client, base+"/v1/scoreboard/"+url.PathEscape(abbr)+"/now", &board); err != nil {
		return nil, fmt.Errorf("hockey: scoreboard: %w", err)
	}

	snap := &types.HockeySnapshot{
		UpdatedAt: time.Now().UTC(),
		TeamAbbr:  abbr,
		TeamName:  h.TeamName,
	}
	live, liveID, latest, next := pickGames(board, abbr, now)
	if live != nil {
		var detail landingResponse
		if err := getJSON(ctx, client, fmt.Sprintf("%s/v1/gamecenter/%d/landing", base, liveID), &detail); err != nil {
			log.Printf("hockey: live game detail: %v", err)
		} else {
			applyLanding(live, detail)
		}
	}
	snap.LiveGame = live
	snap.LatestGame = latest
	snap.NextGame = next
	return snap, nil
}

func getJSON(ctx context.Context, client *http.Client, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("NHL api http %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func pickGames(board scoreboardResponse, abbr string, now time.Time) (live *types.HockeyGame, liveID int64, latest, next *types.HockeyGame) {
	var liveTime, latestTime, nextTime time.Time
	for _, d := range board.GamesByDate {
		for _, g := range d.Games {
			if g.GameType != gameTypeRegular && g.GameType != gameTypePlayoffs {
				continue
			}
			if g.GameScheduleState != "OK" {
				continue
			}
			gameTime, err := time.Parse(time.RFC3339, g.StartTimeUTC)
			if err != nil {
				continue
			}
			converted := convertGame(g, gameTime, abbr)
			if converted == nil {
				continue
			}
			switch {
			case isLive(g.GameState):
				if live == nil || gameTime.After(liveTime) {
					live = converted
					liveID = g.ID
					liveTime = gameTime
				}
			case isCompleted(g.GameState):
				if latest == nil || gameTime.After(latestTime) {
					latest = converted
					latestTime = gameTime
				}
			case isUpcoming(g.GameState, gameTime, now):
				if next == nil || gameTime.Before(nextTime) {
					next = converted
					nextTime = gameTime
				}
			}
		}
	}
	return live, liveID, latest, next
}

func isCompleted(state string) bool {
	return state == "OFF" || state == "FINAL"
}

func isLive(state string) bool {
	return state == "LIVE" || state == "CRIT"
}

func isUpcoming(state string, gameTime, now time.Time) bool {
	if gameTime.Before(now) {
		return false
	}
	return state == "FUT" || state == "PRE"
}

func convertGame(g scoreboardGame, gameTime time.Time, abbr string) *types.HockeyGame {
	var us, them scoreboardTeam
	var homeAway string
	switch abbr {
	case g.HomeTeam.Abbrev:
		us, them, homeAway = g.HomeTeam, g.AwayTeam, "home"
	case g.AwayTeam.Abbrev:
		us, them, homeAway = g.AwayTeam, g.HomeTeam, "away"
	default:
		return nil
	}
	return &types.HockeyGame{
		GameTime:      gameTime,
		Opponent:      them.Name.Default,
		OpponentAbbr:  them.Abbrev,
		HomeAway:      homeAway,
		Venue:         g.Venue.Default,
		Status:        g.GameState,
		IsFinal:       isCompleted(g.GameState),
		IsLive:        isLive(g.GameState),
		TeamScore:     us.Score,
		OpponentScore: them.Score,
		GameType:      g.GameType,
		Period:        g.PeriodDescriptor.Number,
		PeriodType:    g.PeriodDescriptor.PeriodType,
	}
}

func applyLanding(game *types.HockeyGame, detail landingResponse) {
	game.Clock = detail.Clock.TimeRemaining
	game.InIntermission = detail.Clock.InIntermission
	if detail.PeriodDescriptor.Number > 0 {
		game.Period = detail.PeriodDescriptor.Number
		game.PeriodType = detail.PeriodDescriptor.PeriodType
	}
}

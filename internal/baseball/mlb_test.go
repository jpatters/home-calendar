package baseball_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/jpatters/home-calendar/internal/baseball"
	"github.com/jpatters/home-calendar/internal/types"
)

type stubDate struct {
	Date  string     `json:"date"`
	Games []stubGame `json:"games"`
}

type stubGame struct {
	GamePk    int64         `json:"gamePk"`
	GameDate  string        `json:"gameDate"`
	GameType  string        `json:"gameType"`
	Status    stubStatus    `json:"status"`
	Teams     stubTeams     `json:"teams"`
	Venue     stubVenue     `json:"venue"`
	Linescore stubLinescore `json:"linescore"`
}

type stubLinescore struct {
	CurrentInning        int    `json:"currentInning"`
	CurrentInningOrdinal string `json:"currentInningOrdinal"`
	InningState          string `json:"inningState"`
	InningHalf           string `json:"inningHalf"`
	Outs                 int    `json:"outs"`
}

type stubStatus struct {
	AbstractGameState string `json:"abstractGameState"`
	DetailedState     string `json:"detailedState"`
}

type stubTeams struct {
	Home stubSide `json:"home"`
	Away stubSide `json:"away"`
}

type stubSide struct {
	Team  stubTeam `json:"team"`
	Score int      `json:"score"`
}

type stubTeam struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Abbreviation string `json:"abbreviation"`
}

type stubVenue struct {
	Name string `json:"name"`
}

// scheduleStub responds to MLB schedule API requests with the supplied dates.
// It also records the most recent request so tests can inspect query params.
type scheduleStub struct {
	srv       *httptest.Server
	lastQuery url.Values
}

func newScheduleStub(t *testing.T, dates []stubDate) *scheduleStub {
	t.Helper()
	s := &scheduleStub{}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.lastQuery = r.URL.Query()
		resp := map[string]any{
			"totalGames": 0,
			"dates":      dates,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	return s
}

func (s *scheduleStub) URL() string          { return s.srv.URL }
func (s *scheduleStub) Close()               { s.srv.Close() }
func (s *scheduleStub) Client() *http.Client { return s.srv.Client() }

const yankeesID = 147

func yankeesConfig() types.Baseball {
	return types.Baseball{
		Enabled:  true,
		TeamID:   yankeesID,
		TeamName: "New York Yankees",
		TeamAbbr: "NYY",
	}
}

func rfcTime(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return ts
}

func TestSearchReturnsLatestCompletedGameScore(t *testing.T) {
	dates := []stubDate{
		{
			Date: "2026-04-18",
			Games: []stubGame{{
				GamePk: 1, GameDate: "2026-04-18T23:05:00Z", GameType: "R",
				Status: stubStatus{AbstractGameState: "Final", DetailedState: "Final"},
				Teams: stubTeams{
					Home: stubSide{Team: stubTeam{ID: yankeesID, Name: "New York Yankees", Abbreviation: "NYY"}, Score: 2},
					Away: stubSide{Team: stubTeam{ID: 111, Name: "Boston Red Sox", Abbreviation: "BOS"}, Score: 4},
				},
				Venue: stubVenue{Name: "Yankee Stadium"},
			}},
		},
		{
			Date: "2026-04-20",
			Games: []stubGame{{
				GamePk: 2, GameDate: "2026-04-20T23:05:00Z", GameType: "R",
				Status: stubStatus{AbstractGameState: "Final", DetailedState: "Final"},
				Teams: stubTeams{
					Home: stubSide{Team: stubTeam{ID: yankeesID, Name: "New York Yankees", Abbreviation: "NYY"}, Score: 5},
					Away: stubSide{Team: stubTeam{ID: 111, Name: "Boston Red Sox", Abbreviation: "BOS"}, Score: 3},
				},
				Venue: stubVenue{Name: "Yankee Stadium"},
			}},
		},
		{
			Date: "2026-04-22",
			Games: []stubGame{{
				GamePk: 3, GameDate: "2026-04-22T23:05:00Z", GameType: "R",
				Status: stubStatus{AbstractGameState: "Preview", DetailedState: "Scheduled"},
				Teams: stubTeams{
					Home: stubSide{Team: stubTeam{ID: 121, Name: "New York Mets", Abbreviation: "NYM"}},
					Away: stubSide{Team: stubTeam{ID: yankeesID, Name: "New York Yankees", Abbreviation: "NYY"}},
				},
				Venue: stubVenue{Name: "Citi Field"},
			}},
		},
	}
	stub := newScheduleStub(t, dates)
	defer stub.Close()
	now := rfcTime(t, "2026-04-21T12:00:00Z")

	snap, err := baseball.Search(context.Background(), stub.Client(), stub.URL(), yankeesConfig(), now)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if snap == nil || snap.LatestGame == nil {
		t.Fatalf("expected LatestGame, got snap=%+v", snap)
	}
	got := snap.LatestGame
	if !got.IsFinal {
		t.Errorf("LatestGame.IsFinal = false, want true")
	}
	if got.TeamScore != 5 {
		t.Errorf("TeamScore = %d, want 5", got.TeamScore)
	}
	if got.OpponentScore != 3 {
		t.Errorf("OpponentScore = %d, want 3", got.OpponentScore)
	}
	if got.Opponent != "Boston Red Sox" {
		t.Errorf("Opponent = %q, want Boston Red Sox", got.Opponent)
	}
	if got.HomeAway != "home" {
		t.Errorf("HomeAway = %q, want home", got.HomeAway)
	}
	wantTime := rfcTime(t, "2026-04-20T23:05:00Z")
	if !got.GameTime.Equal(wantTime) {
		t.Errorf("GameTime = %v, want %v", got.GameTime, wantTime)
	}
}

func TestSearchReturnsNextUpcomingGame(t *testing.T) {
	// A game scheduled in the past (4-21, before now=4-22) must be ignored.
	// Between two future scheduled games (4-23 Mets, 4-25 Dodgers), the 4-23
	// game must win because it's nearest in time.
	dates := []stubDate{
		{
			Date: "2026-04-20",
			Games: []stubGame{{
				GamePk: 1, GameDate: "2026-04-20T23:05:00Z", GameType: "R",
				Status: stubStatus{AbstractGameState: "Final", DetailedState: "Final"},
				Teams: stubTeams{
					Home: stubSide{Team: stubTeam{ID: yankeesID, Name: "New York Yankees", Abbreviation: "NYY"}, Score: 5},
					Away: stubSide{Team: stubTeam{ID: 111, Name: "Boston Red Sox", Abbreviation: "BOS"}, Score: 3},
				},
				Venue: stubVenue{Name: "Yankee Stadium"},
			}},
		},
		{
			// Scheduled in the past — must not be picked as "next".
			Date: "2026-04-21",
			Games: []stubGame{{
				GamePk: 99, GameDate: "2026-04-21T00:05:00Z", GameType: "R",
				Status: stubStatus{AbstractGameState: "Preview", DetailedState: "Scheduled"},
				Teams: stubTeams{
					Home: stubSide{Team: stubTeam{ID: yankeesID, Name: "New York Yankees", Abbreviation: "NYY"}},
					Away: stubSide{Team: stubTeam{ID: 140, Name: "Texas Rangers", Abbreviation: "TEX"}},
				},
				Venue: stubVenue{Name: "Yankee Stadium"},
			}},
		},
		{
			Date: "2026-04-23",
			Games: []stubGame{{
				GamePk: 2, GameDate: "2026-04-23T23:05:00Z", GameType: "R",
				Status: stubStatus{AbstractGameState: "Preview", DetailedState: "Scheduled"},
				Teams: stubTeams{
					Home: stubSide{Team: stubTeam{ID: 121, Name: "New York Mets", Abbreviation: "NYM"}},
					Away: stubSide{Team: stubTeam{ID: yankeesID, Name: "New York Yankees", Abbreviation: "NYY"}},
				},
				Venue: stubVenue{Name: "Citi Field"},
			}},
		},
		{
			Date: "2026-04-25",
			Games: []stubGame{{
				GamePk: 3, GameDate: "2026-04-25T23:05:00Z", GameType: "R",
				Status: stubStatus{AbstractGameState: "Preview", DetailedState: "Scheduled"},
				Teams: stubTeams{
					Home: stubSide{Team: stubTeam{ID: yankeesID, Name: "New York Yankees", Abbreviation: "NYY"}},
					Away: stubSide{Team: stubTeam{ID: 119, Name: "Los Angeles Dodgers", Abbreviation: "LAD"}},
				},
				Venue: stubVenue{Name: "Yankee Stadium"},
			}},
		},
	}
	stub := newScheduleStub(t, dates)
	defer stub.Close()
	now := rfcTime(t, "2026-04-22T12:00:00Z")

	snap, err := baseball.Search(context.Background(), stub.Client(), stub.URL(), yankeesConfig(), now)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if snap == nil || snap.NextGame == nil {
		t.Fatalf("expected NextGame, got snap=%+v", snap)
	}
	got := snap.NextGame
	if got.Opponent != "New York Mets" {
		t.Errorf("Opponent = %q, want New York Mets", got.Opponent)
	}
	if got.HomeAway != "away" {
		t.Errorf("HomeAway = %q, want away", got.HomeAway)
	}
	if got.Venue != "Citi Field" {
		t.Errorf("Venue = %q, want Citi Field", got.Venue)
	}
	wantTime := rfcTime(t, "2026-04-23T23:05:00Z")
	if !got.GameTime.Equal(wantTime) {
		t.Errorf("GameTime = %v, want %v", got.GameTime, wantTime)
	}
	if got.IsFinal {
		t.Errorf("IsFinal = true for upcoming game, want false")
	}
}

func TestSearchFiltersOutSpringTrainingAndExhibition(t *testing.T) {
	dates := []stubDate{
		{
			Date: "2026-04-19",
			Games: []stubGame{{
				GamePk: 1, GameDate: "2026-04-19T23:05:00Z", GameType: "S", // spring training
				Status: stubStatus{AbstractGameState: "Final", DetailedState: "Final"},
				Teams: stubTeams{
					Home: stubSide{Team: stubTeam{ID: yankeesID, Name: "New York Yankees", Abbreviation: "NYY"}, Score: 9},
					Away: stubSide{Team: stubTeam{ID: 111, Name: "Boston Red Sox", Abbreviation: "BOS"}, Score: 2},
				},
			}},
		},
		{
			Date: "2026-04-20",
			Games: []stubGame{{
				GamePk: 2, GameDate: "2026-04-20T23:05:00Z", GameType: "R", // regular season
				Status: stubStatus{AbstractGameState: "Final", DetailedState: "Final"},
				Teams: stubTeams{
					Home: stubSide{Team: stubTeam{ID: yankeesID, Name: "New York Yankees", Abbreviation: "NYY"}, Score: 5},
					Away: stubSide{Team: stubTeam{ID: 111, Name: "Boston Red Sox", Abbreviation: "BOS"}, Score: 3},
				},
			}},
		},
		{
			Date: "2026-04-23",
			Games: []stubGame{{
				GamePk: 3, GameDate: "2026-04-23T23:05:00Z", GameType: "E", // exhibition
				Status: stubStatus{AbstractGameState: "Preview", DetailedState: "Scheduled"},
				Teams: stubTeams{
					Home: stubSide{Team: stubTeam{ID: 121, Name: "New York Mets", Abbreviation: "NYM"}},
					Away: stubSide{Team: stubTeam{ID: yankeesID, Name: "New York Yankees", Abbreviation: "NYY"}},
				},
			}},
		},
		{
			Date: "2026-04-25",
			Games: []stubGame{{
				GamePk: 4, GameDate: "2026-04-25T23:05:00Z", GameType: "R",
				Status: stubStatus{AbstractGameState: "Preview", DetailedState: "Scheduled"},
				Teams: stubTeams{
					Home: stubSide{Team: stubTeam{ID: yankeesID, Name: "New York Yankees", Abbreviation: "NYY"}},
					Away: stubSide{Team: stubTeam{ID: 119, Name: "Los Angeles Dodgers", Abbreviation: "LAD"}},
				},
			}},
		},
	}
	stub := newScheduleStub(t, dates)
	defer stub.Close()
	now := rfcTime(t, "2026-04-22T00:00:00Z")

	snap, err := baseball.Search(context.Background(), stub.Client(), stub.URL(), yankeesConfig(), now)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if snap.LatestGame == nil {
		t.Fatalf("expected LatestGame to be the R game, got nil")
	}
	if snap.LatestGame.Opponent != "Boston Red Sox" || snap.LatestGame.TeamScore != 5 {
		t.Errorf("LatestGame picked wrong game: %+v (expected the R-type 5-3 game)", snap.LatestGame)
	}
	if snap.NextGame == nil {
		t.Fatalf("expected NextGame to skip exhibition and pick the R game")
	}
	if snap.NextGame.Opponent != "Los Angeles Dodgers" {
		t.Errorf("NextGame = %+v, want Dodgers game (R), got opponent %q", snap.NextGame, snap.NextGame.Opponent)
	}
}

func TestSearchIncludesPlayoffGames(t *testing.T) {
	dates := []stubDate{
		{
			Date: "2026-10-10",
			Games: []stubGame{{
				GamePk: 1, GameDate: "2026-10-10T23:05:00Z", GameType: "D", // division series
				Status: stubStatus{AbstractGameState: "Final", DetailedState: "Final"},
				Teams: stubTeams{
					Home: stubSide{Team: stubTeam{ID: yankeesID, Name: "New York Yankees", Abbreviation: "NYY"}, Score: 6},
					Away: stubSide{Team: stubTeam{ID: 117, Name: "Houston Astros", Abbreviation: "HOU"}, Score: 2},
				},
			}},
		},
		{
			Date: "2026-10-14",
			Games: []stubGame{{
				GamePk: 2, GameDate: "2026-10-14T23:05:00Z", GameType: "L", // league series
				Status: stubStatus{AbstractGameState: "Preview", DetailedState: "Scheduled"},
				Teams: stubTeams{
					Home: stubSide{Team: stubTeam{ID: yankeesID, Name: "New York Yankees", Abbreviation: "NYY"}},
					Away: stubSide{Team: stubTeam{ID: 140, Name: "Texas Rangers", Abbreviation: "TEX"}},
				},
			}},
		},
	}
	stub := newScheduleStub(t, dates)
	defer stub.Close()
	now := rfcTime(t, "2026-10-12T00:00:00Z")

	snap, err := baseball.Search(context.Background(), stub.Client(), stub.URL(), yankeesConfig(), now)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if snap.LatestGame == nil {
		t.Fatalf("expected playoff Final to appear as LatestGame")
	}
	if snap.NextGame == nil {
		t.Fatalf("expected playoff Preview to appear as NextGame")
	}
}

func TestSearchHandlesDoubleHeader(t *testing.T) {
	dates := []stubDate{
		{
			Date: "2026-04-19",
			Games: []stubGame{
				{
					GamePk: 1, GameDate: "2026-04-19T17:05:00Z", GameType: "R",
					Status: stubStatus{AbstractGameState: "Final", DetailedState: "Final"},
					Teams: stubTeams{
						Home: stubSide{Team: stubTeam{ID: yankeesID, Name: "New York Yankees", Abbreviation: "NYY"}, Score: 1},
						Away: stubSide{Team: stubTeam{ID: 111, Name: "Boston Red Sox", Abbreviation: "BOS"}, Score: 2},
					},
				},
				{
					GamePk: 2, GameDate: "2026-04-19T23:05:00Z", GameType: "R",
					Status: stubStatus{AbstractGameState: "Final", DetailedState: "Final"},
					Teams: stubTeams{
						Home: stubSide{Team: stubTeam{ID: yankeesID, Name: "New York Yankees", Abbreviation: "NYY"}, Score: 7},
						Away: stubSide{Team: stubTeam{ID: 111, Name: "Boston Red Sox", Abbreviation: "BOS"}, Score: 4},
					},
				},
			},
		},
	}
	stub := newScheduleStub(t, dates)
	defer stub.Close()
	now := rfcTime(t, "2026-04-20T00:00:00Z")

	snap, err := baseball.Search(context.Background(), stub.Client(), stub.URL(), yankeesConfig(), now)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if snap.LatestGame == nil {
		t.Fatalf("expected LatestGame")
	}
	if snap.LatestGame.TeamScore != 7 || snap.LatestGame.OpponentScore != 4 {
		t.Errorf("LatestGame picked wrong double-header game: %+v (expected the later 7-4 game)", snap.LatestGame)
	}
}

func TestSearchReturnsNilLatestWhenNoCompleted(t *testing.T) {
	dates := []stubDate{
		{
			Date: "2026-04-25",
			Games: []stubGame{{
				GamePk: 1, GameDate: "2026-04-25T23:05:00Z", GameType: "R",
				Status: stubStatus{AbstractGameState: "Preview", DetailedState: "Scheduled"},
				Teams: stubTeams{
					Home: stubSide{Team: stubTeam{ID: yankeesID, Name: "New York Yankees", Abbreviation: "NYY"}},
					Away: stubSide{Team: stubTeam{ID: 111, Name: "Boston Red Sox", Abbreviation: "BOS"}},
				},
			}},
		},
	}
	stub := newScheduleStub(t, dates)
	defer stub.Close()
	now := rfcTime(t, "2026-04-22T00:00:00Z")

	snap, err := baseball.Search(context.Background(), stub.Client(), stub.URL(), yankeesConfig(), now)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if snap.LatestGame != nil {
		t.Errorf("LatestGame = %+v, want nil when no completed game", snap.LatestGame)
	}
	if snap.NextGame == nil {
		t.Errorf("NextGame should still populate independently")
	}
}

func TestSearchReturnsNilNextWhenNoUpcoming(t *testing.T) {
	dates := []stubDate{
		{
			Date: "2026-04-18",
			Games: []stubGame{{
				GamePk: 1, GameDate: "2026-04-18T23:05:00Z", GameType: "R",
				Status: stubStatus{AbstractGameState: "Final", DetailedState: "Final"},
				Teams: stubTeams{
					Home: stubSide{Team: stubTeam{ID: yankeesID, Name: "New York Yankees", Abbreviation: "NYY"}, Score: 3},
					Away: stubSide{Team: stubTeam{ID: 111, Name: "Boston Red Sox", Abbreviation: "BOS"}, Score: 2},
				},
			}},
		},
	}
	stub := newScheduleStub(t, dates)
	defer stub.Close()
	now := rfcTime(t, "2026-04-22T00:00:00Z")

	snap, err := baseball.Search(context.Background(), stub.Client(), stub.URL(), yankeesConfig(), now)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if snap.NextGame != nil {
		t.Errorf("NextGame = %+v, want nil when no upcoming", snap.NextGame)
	}
	if snap.LatestGame == nil {
		t.Errorf("LatestGame should still populate independently")
	}
}

func TestSearchSurfacesLiveGame(t *testing.T) {
	// A game currently in progress should populate snap.LiveGame with the
	// current score, inning, and outs from the linescore hydration.
	dates := []stubDate{
		{
			Date: "2026-04-22",
			Games: []stubGame{{
				GamePk: 1, GameDate: "2026-04-22T23:00:00Z", GameType: "R",
				Status: stubStatus{AbstractGameState: "Live", DetailedState: "In Progress"},
				Teams: stubTeams{
					Home: stubSide{Team: stubTeam{ID: yankeesID, Name: "New York Yankees", Abbreviation: "NYY"}, Score: 3},
					Away: stubSide{Team: stubTeam{ID: 111, Name: "Boston Red Sox", Abbreviation: "BOS"}, Score: 1},
				},
				Venue: stubVenue{Name: "Yankee Stadium"},
				Linescore: stubLinescore{
					CurrentInning: 5, CurrentInningOrdinal: "5th",
					InningState: "Top", InningHalf: "Top", Outs: 2,
				},
			}},
		},
	}
	stub := newScheduleStub(t, dates)
	defer stub.Close()
	now := rfcTime(t, "2026-04-22T23:30:00Z")

	snap, err := baseball.Search(context.Background(), stub.Client(), stub.URL(), yankeesConfig(), now)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if snap.LiveGame == nil {
		t.Fatalf("expected LiveGame, got nil")
	}
	got := snap.LiveGame
	if !got.IsLive {
		t.Errorf("LiveGame.IsLive = false, want true")
	}
	if got.IsFinal {
		t.Errorf("LiveGame.IsFinal = true, want false")
	}
	if got.TeamScore != 3 {
		t.Errorf("LiveGame.TeamScore = %d, want 3", got.TeamScore)
	}
	if got.OpponentScore != 1 {
		t.Errorf("LiveGame.OpponentScore = %d, want 1", got.OpponentScore)
	}
	if got.Inning != 5 {
		t.Errorf("LiveGame.Inning = %d, want 5", got.Inning)
	}
	if got.InningHalf != "top" {
		t.Errorf("LiveGame.InningHalf = %q, want top", got.InningHalf)
	}
	if got.Outs != 2 {
		t.Errorf("LiveGame.Outs = %d, want 2", got.Outs)
	}
}

func TestSearchLiveGameNotDuplicatedAsLatestOrNext(t *testing.T) {
	// When a live game is also present alongside a recent Final and an upcoming
	// Preview, each must occupy its own snapshot slot. The live game must not
	// also appear as Latest or Next.
	dates := []stubDate{
		{
			Date: "2026-04-20",
			Games: []stubGame{{
				GamePk: 1, GameDate: "2026-04-20T23:05:00Z", GameType: "R",
				Status: stubStatus{AbstractGameState: "Final", DetailedState: "Final"},
				Teams: stubTeams{
					Home: stubSide{Team: stubTeam{ID: yankeesID, Name: "New York Yankees", Abbreviation: "NYY"}, Score: 5},
					Away: stubSide{Team: stubTeam{ID: 111, Name: "Boston Red Sox", Abbreviation: "BOS"}, Score: 3},
				},
			}},
		},
		{
			Date: "2026-04-22",
			Games: []stubGame{{
				GamePk: 2, GameDate: "2026-04-22T23:00:00Z", GameType: "R",
				Status: stubStatus{AbstractGameState: "Live", DetailedState: "In Progress"},
				Teams: stubTeams{
					Home: stubSide{Team: stubTeam{ID: yankeesID, Name: "New York Yankees", Abbreviation: "NYY"}, Score: 2},
					Away: stubSide{Team: stubTeam{ID: 111, Name: "Boston Red Sox", Abbreviation: "BOS"}, Score: 1},
				},
				Linescore: stubLinescore{CurrentInning: 4, InningHalf: "Bottom", Outs: 1},
			}},
		},
		{
			Date: "2026-04-24",
			Games: []stubGame{{
				GamePk: 3, GameDate: "2026-04-24T23:05:00Z", GameType: "R",
				Status: stubStatus{AbstractGameState: "Preview", DetailedState: "Scheduled"},
				Teams: stubTeams{
					Home: stubSide{Team: stubTeam{ID: 121, Name: "New York Mets", Abbreviation: "NYM"}},
					Away: stubSide{Team: stubTeam{ID: yankeesID, Name: "New York Yankees", Abbreviation: "NYY"}},
				},
				Venue: stubVenue{Name: "Citi Field"},
			}},
		},
	}
	stub := newScheduleStub(t, dates)
	defer stub.Close()
	now := rfcTime(t, "2026-04-22T23:30:00Z")

	snap, err := baseball.Search(context.Background(), stub.Client(), stub.URL(), yankeesConfig(), now)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if snap.LiveGame == nil {
		t.Fatalf("expected LiveGame populated")
	}
	if snap.LatestGame == nil || snap.LatestGame.TeamScore != 5 {
		t.Errorf("LatestGame should be the 5-3 Final, got %+v", snap.LatestGame)
	}
	if snap.NextGame == nil || snap.NextGame.Opponent != "New York Mets" {
		t.Errorf("NextGame should be the Mets Preview, got %+v", snap.NextGame)
	}
	// The live game must not also appear as latest or next.
	if snap.LatestGame != nil && snap.LatestGame.IsLive {
		t.Errorf("Live game leaked into LatestGame: %+v", snap.LatestGame)
	}
	if snap.NextGame != nil && snap.NextGame.IsLive {
		t.Errorf("Live game leaked into NextGame: %+v", snap.NextGame)
	}
}

func TestSearchIgnoresPostponedAsCompleted(t *testing.T) {
	dates := []stubDate{
		{
			Date: "2026-04-18",
			Games: []stubGame{{
				GamePk: 1, GameDate: "2026-04-18T23:05:00Z", GameType: "R",
				Status: stubStatus{AbstractGameState: "Final", DetailedState: "Postponed"},
				Teams: stubTeams{
					Home: stubSide{Team: stubTeam{ID: yankeesID, Name: "New York Yankees", Abbreviation: "NYY"}},
					Away: stubSide{Team: stubTeam{ID: 111, Name: "Boston Red Sox", Abbreviation: "BOS"}},
				},
			}},
		},
		{
			Date: "2026-04-19",
			Games: []stubGame{{
				GamePk: 2, GameDate: "2026-04-19T23:05:00Z", GameType: "R",
				Status: stubStatus{AbstractGameState: "Final", DetailedState: "Final"},
				Teams: stubTeams{
					Home: stubSide{Team: stubTeam{ID: yankeesID, Name: "New York Yankees", Abbreviation: "NYY"}, Score: 4},
					Away: stubSide{Team: stubTeam{ID: 111, Name: "Boston Red Sox", Abbreviation: "BOS"}, Score: 1},
				},
			}},
		},
	}
	stub := newScheduleStub(t, dates)
	defer stub.Close()
	now := rfcTime(t, "2026-04-22T00:00:00Z")

	snap, err := baseball.Search(context.Background(), stub.Client(), stub.URL(), yankeesConfig(), now)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if snap.LatestGame == nil {
		t.Fatalf("expected LatestGame to be the real Final game")
	}
	if snap.LatestGame.TeamScore != 4 || snap.LatestGame.OpponentScore != 1 {
		t.Errorf("LatestGame = %+v, want the 4-1 Final game (postponed should be skipped)", snap.LatestGame)
	}
}

func TestSearchReturnsErrorOnUpstreamFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	snap, err := baseball.Search(context.Background(), srv.Client(), srv.URL, yankeesConfig(), time.Now())
	if err == nil {
		t.Fatalf("expected error on 500, got snap=%+v", snap)
	}
	if snap != nil {
		t.Errorf("expected nil snapshot on error, got %+v", snap)
	}
}

func TestSearchSendsRequiredAPIParams(t *testing.T) {
	// MLB's API returns wrong-team data if sportId/teamId are wrong, so the
	// contract with the upstream service demands these specific params.
	stub := newScheduleStub(t, nil)
	defer stub.Close()
	now := rfcTime(t, "2026-04-22T00:00:00Z")

	_, err := baseball.Search(context.Background(), stub.Client(), stub.URL(), yankeesConfig(), now)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	q := stub.lastQuery
	if q.Get("sportId") != "1" {
		t.Errorf("sportId = %q, want 1", q.Get("sportId"))
	}
	if q.Get("teamId") != "147" {
		t.Errorf("teamId = %q, want 147", q.Get("teamId"))
	}
}

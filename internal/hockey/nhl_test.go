package hockey_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jpatters/home-calendar/internal/hockey"
	"github.com/jpatters/home-calendar/internal/types"
)

// stubNHL serves a canned scoreboard at /v1/scoreboard/{abbrev}/now and
// canned gamecenter landings at /v1/gamecenter/{id}/landing, the two
// endpoints the hockey package reads. It records request paths so tests can
// check which team was asked for.
type stubNHL struct {
	srv      *httptest.Server
	mu       sync.Mutex
	games    []map[string]any
	landings map[string]map[string]any
	landing  int
	paths    []string
}

func newStubNHL(t *testing.T, games []map[string]any) *stubNHL {
	t.Helper()
	s := &stubNHL{games: games, landings: map[string]map[string]any{}, landing: http.StatusOK}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.paths = append(s.paths, r.URL.Path)
		games := s.games
		landings := s.landings
		landingStatus := s.landing
		s.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/v1/scoreboard/"):
			// Group games by their gameDate the way the real API does.
			byDate := map[string][]map[string]any{}
			var order []string
			for _, g := range games {
				d := g["gameDate"].(string)
				if _, seen := byDate[d]; !seen {
					order = append(order, d)
				}
				byDate[d] = append(byDate[d], g)
			}
			var days []map[string]any
			for _, d := range order {
				days = append(days, map[string]any{"date": d, "games": byDate[d]})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"focusedDate": "2026-10-21",
				"gamesByDate": days,
			})
		case strings.HasPrefix(r.URL.Path, "/v1/gamecenter/"):
			if landingStatus != http.StatusOK {
				http.Error(w, "boom", landingStatus)
				return
			}
			parts := strings.Split(r.URL.Path, "/")
			body, ok := landings[parts[3]]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(body)
		default:
			http.NotFound(w, r)
		}
	}))
	return s
}

func (s *stubNHL) URL() string          { return s.srv.URL }
func (s *stubNHL) Client() *http.Client { return s.srv.Client() }
func (s *stubNHL) Close()               { s.srv.Close() }

func (s *stubNHL) Paths() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.paths...)
}

func (s *stubNHL) SetLanding(id string, body map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.landings[id] = body
}

func (s *stubNHL) FailLandings() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.landing = http.StatusInternalServerError
}

func team(abbrev, name string, score int) map[string]any {
	return map[string]any{
		"id":     1,
		"abbrev": abbrev,
		"name":   map[string]any{"default": name},
		"score":  score,
	}
}

// futTeam is a team entry on a game that hasn't started: no score key.
func futTeam(abbrev, name string) map[string]any {
	return map[string]any{
		"id":     1,
		"abbrev": abbrev,
		"name":   map[string]any{"default": name},
	}
}

type gameOpts struct {
	id            int64
	date          string // YYYY-MM-DD
	start         string // RFC3339
	gameType      int
	state         string
	scheduleState string
	away, home    map[string]any
	venue         string
	period        int
	periodType    string
}

func game(o gameOpts) map[string]any {
	if o.scheduleState == "" {
		o.scheduleState = "OK"
	}
	if o.gameType == 0 {
		o.gameType = 2
	}
	g := map[string]any{
		"id":                o.id,
		"gameDate":          o.date,
		"startTimeUTC":      o.start,
		"gameType":          o.gameType,
		"gameState":         o.state,
		"gameScheduleState": o.scheduleState,
		"venue":             map[string]any{"default": o.venue},
		"awayTeam":          o.away,
		"homeTeam":          o.home,
	}
	if o.period > 0 {
		g["period"] = o.period
		g["periodDescriptor"] = map[string]any{
			"number":               o.period,
			"periodType":           o.periodType,
			"maxRegulationPeriods": 3,
		}
	}
	return g
}

func landing(state string, period int, periodType, clock string, intermission bool, awayScore, homeScore int) map[string]any {
	return map[string]any{
		"gameState": state,
		"clock": map[string]any{
			"timeRemaining":    clock,
			"secondsRemaining": 0,
			"running":          !intermission,
			"inIntermission":   intermission,
		},
		"periodDescriptor": map[string]any{
			"number":     period,
			"periodType": periodType,
		},
		"awayTeam": map[string]any{"score": awayScore},
		"homeTeam": map[string]any{"score": homeScore},
	}
}

func leafsConfig() types.Hockey {
	return types.Hockey{Enabled: true, TeamAbbr: "TOR", TeamName: "Toronto Maple Leafs"}
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
	games := []map[string]any{
		game(gameOpts{id: 1, date: "2026-10-17", start: "2026-10-17T23:00:00Z", state: "OFF",
			away: team("NYI", "New York Islanders", 4), home: team("TOR", "Toronto Maple Leafs", 1),
			venue: "Scotiabank Arena", period: 3, periodType: "REG"}),
		game(gameOpts{id: 2, date: "2026-10-20", start: "2026-10-20T23:00:00Z", state: "OFF",
			away: team("TOR", "Toronto Maple Leafs", 6), home: team("CBJ", "Columbus Blue Jackets", 2),
			venue: "Nationwide Arena", period: 3, periodType: "REG"}),
		game(gameOpts{id: 3, date: "2026-10-22", start: "2026-10-22T23:00:00Z", state: "FUT",
			away: futTeam("NYR", "New York Rangers"), home: futTeam("TOR", "Toronto Maple Leafs"),
			venue: "Scotiabank Arena"}),
	}
	stub := newStubNHL(t, games)
	defer stub.Close()
	now := rfcTime(t, "2026-10-21T12:00:00Z")

	snap, err := hockey.Search(context.Background(), stub.Client(), stub.URL(), leafsConfig(), now)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if snap == nil || snap.LatestGame == nil {
		t.Fatalf("expected LatestGame, got snap=%+v", snap)
	}
	got := snap.LatestGame
	if !got.IsFinal {
		t.Errorf("IsFinal = false, want true")
	}
	if got.TeamScore != 6 || got.OpponentScore != 2 {
		t.Errorf("score = %d-%d, want 6-2", got.TeamScore, got.OpponentScore)
	}
	if got.Opponent != "Columbus Blue Jackets" {
		t.Errorf("Opponent = %q, want Columbus Blue Jackets", got.Opponent)
	}
	if got.OpponentAbbr != "CBJ" {
		t.Errorf("OpponentAbbr = %q, want CBJ", got.OpponentAbbr)
	}
	if got.HomeAway != "away" {
		t.Errorf("HomeAway = %q, want away", got.HomeAway)
	}
	if got.Venue != "Nationwide Arena" {
		t.Errorf("Venue = %q, want Nationwide Arena", got.Venue)
	}
	if got.Period != 3 || got.PeriodType != "REG" {
		t.Errorf("Period=%d PeriodType=%q, want 3 REG", got.Period, got.PeriodType)
	}
	if !got.GameTime.Equal(rfcTime(t, "2026-10-20T23:00:00Z")) {
		t.Errorf("GameTime = %v, want 2026-10-20T23:00:00Z", got.GameTime)
	}
	if snap.TeamAbbr != "TOR" {
		t.Errorf("snapshot TeamAbbr = %q, want TOR", snap.TeamAbbr)
	}
}

func TestSearchTreatsFinalStateAsCompleted(t *testing.T) {
	games := []map[string]any{
		game(gameOpts{id: 1, date: "2026-10-20", start: "2026-10-20T23:00:00Z", state: "FINAL",
			away: team("TOR", "Toronto Maple Leafs", 2), home: team("CBJ", "Columbus Blue Jackets", 5),
			venue: "Nationwide Arena", period: 3, periodType: "REG"}),
	}
	stub := newStubNHL(t, games)
	defer stub.Close()

	snap, err := hockey.Search(context.Background(), stub.Client(), stub.URL(), leafsConfig(), rfcTime(t, "2026-10-21T12:00:00Z"))
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if snap.LatestGame == nil || !snap.LatestGame.IsFinal {
		t.Fatalf("expected FINAL game as LatestGame, got %+v", snap.LatestGame)
	}
	if snap.LatestGame.TeamScore != 2 || snap.LatestGame.OpponentScore != 5 {
		t.Errorf("score = %d-%d, want 2-5", snap.LatestGame.TeamScore, snap.LatestGame.OpponentScore)
	}
}

func TestSearchReturnsNextUpcomingGame(t *testing.T) {
	// A FUT game whose start is before now must be ignored; between two
	// future games the nearer one wins.
	games := []map[string]any{
		game(gameOpts{id: 1, date: "2026-10-20", start: "2026-10-20T23:00:00Z", state: "FUT",
			away: futTeam("TOR", "Toronto Maple Leafs"), home: futTeam("CBJ", "Columbus Blue Jackets"),
			venue: "Nationwide Arena"}),
		game(gameOpts{id: 2, date: "2026-10-24", start: "2026-10-24T23:00:00Z", state: "FUT",
			away: futTeam("TOR", "Toronto Maple Leafs"), home: futTeam("EDM", "Edmonton Oilers"),
			venue: "Rogers Place"}),
		game(gameOpts{id: 3, date: "2026-10-22", start: "2026-10-22T23:00:00Z", state: "FUT",
			away: futTeam("NYR", "New York Rangers"), home: futTeam("TOR", "Toronto Maple Leafs"),
			venue: "Scotiabank Arena"}),
	}
	stub := newStubNHL(t, games)
	defer stub.Close()
	now := rfcTime(t, "2026-10-21T12:00:00Z")

	snap, err := hockey.Search(context.Background(), stub.Client(), stub.URL(), leafsConfig(), now)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if snap.NextGame == nil {
		t.Fatal("expected NextGame")
	}
	got := snap.NextGame
	if got.Opponent != "New York Rangers" {
		t.Errorf("Opponent = %q, want New York Rangers", got.Opponent)
	}
	if got.HomeAway != "home" {
		t.Errorf("HomeAway = %q, want home", got.HomeAway)
	}
	if got.Venue != "Scotiabank Arena" {
		t.Errorf("Venue = %q, want Scotiabank Arena", got.Venue)
	}
	if got.IsFinal || got.IsLive {
		t.Errorf("upcoming game flagged final=%v live=%v", got.IsFinal, got.IsLive)
	}
	if !got.GameTime.Equal(rfcTime(t, "2026-10-22T23:00:00Z")) {
		t.Errorf("GameTime = %v, want 2026-10-22T23:00:00Z", got.GameTime)
	}
}

func TestSearchTreatsPreGameStateAsUpcoming(t *testing.T) {
	games := []map[string]any{
		game(gameOpts{id: 1, date: "2026-10-21", start: "2026-10-21T23:00:00Z", state: "PRE",
			away: futTeam("NYR", "New York Rangers"), home: futTeam("TOR", "Toronto Maple Leafs"),
			venue: "Scotiabank Arena"}),
	}
	stub := newStubNHL(t, games)
	defer stub.Close()

	snap, err := hockey.Search(context.Background(), stub.Client(), stub.URL(), leafsConfig(), rfcTime(t, "2026-10-21T22:40:00Z"))
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if snap.NextGame == nil || snap.NextGame.Opponent != "New York Rangers" {
		t.Fatalf("expected PRE game as NextGame, got %+v", snap.NextGame)
	}
	if snap.LiveGame != nil {
		t.Errorf("PRE game must not be reported live: %+v", snap.LiveGame)
	}
}

func TestSearchFiltersOutPreseasonButKeepsPlayoffs(t *testing.T) {
	games := []map[string]any{
		// Preseason final, more recent than the regular-season one: must be ignored.
		game(gameOpts{id: 1, date: "2026-10-20", start: "2026-10-20T23:00:00Z", state: "FINAL", gameType: 1,
			away: team("TOR", "Toronto Maple Leafs", 9), home: team("OTT", "Ottawa Senators", 0),
			venue: "Canadian Tire Centre", period: 3, periodType: "REG"}),
		game(gameOpts{id: 2, date: "2026-10-18", start: "2026-10-18T23:00:00Z", state: "OFF", gameType: 2,
			away: team("TOR", "Toronto Maple Leafs", 2), home: team("CBJ", "Columbus Blue Jackets", 1),
			venue: "Nationwide Arena", period: 3, periodType: "REG"}),
		// Preseason upcoming, nearer than the playoff game: must be ignored.
		game(gameOpts{id: 3, date: "2026-10-22", start: "2026-10-22T23:00:00Z", state: "FUT", gameType: 1,
			away: futTeam("MTL", "Montreal Canadiens"), home: futTeam("TOR", "Toronto Maple Leafs"),
			venue: "Scotiabank Arena"}),
		game(gameOpts{id: 4, date: "2026-10-25", start: "2026-10-25T23:00:00Z", state: "FUT", gameType: 3,
			away: futTeam("BOS", "Boston Bruins"), home: futTeam("TOR", "Toronto Maple Leafs"),
			venue: "Scotiabank Arena"}),
	}
	stub := newStubNHL(t, games)
	defer stub.Close()

	snap, err := hockey.Search(context.Background(), stub.Client(), stub.URL(), leafsConfig(), rfcTime(t, "2026-10-21T12:00:00Z"))
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if snap.LatestGame == nil || snap.LatestGame.Opponent != "Columbus Blue Jackets" {
		t.Errorf("LatestGame = %+v, want the regular-season Columbus game", snap.LatestGame)
	}
	if snap.NextGame == nil || snap.NextGame.Opponent != "Boston Bruins" {
		t.Errorf("NextGame = %+v, want the playoff Boston game", snap.NextGame)
	}
	if snap.NextGame != nil && snap.NextGame.GameType != 3 {
		t.Errorf("NextGame.GameType = %d, want 3", snap.NextGame.GameType)
	}
}

func TestSearchSkipsPostponedGames(t *testing.T) {
	games := []map[string]any{
		game(gameOpts{id: 1, date: "2026-10-22", start: "2026-10-22T23:00:00Z", state: "FUT", scheduleState: "PPD",
			away: futTeam("NYR", "New York Rangers"), home: futTeam("TOR", "Toronto Maple Leafs"),
			venue: "Scotiabank Arena"}),
		game(gameOpts{id: 2, date: "2026-10-24", start: "2026-10-24T23:00:00Z", state: "FUT",
			away: futTeam("TOR", "Toronto Maple Leafs"), home: futTeam("EDM", "Edmonton Oilers"),
			venue: "Rogers Place"}),
	}
	stub := newStubNHL(t, games)
	defer stub.Close()

	snap, err := hockey.Search(context.Background(), stub.Client(), stub.URL(), leafsConfig(), rfcTime(t, "2026-10-21T12:00:00Z"))
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if snap.NextGame == nil || snap.NextGame.Opponent != "Edmonton Oilers" {
		t.Errorf("NextGame = %+v, want Edmonton (postponed Rangers game skipped)", snap.NextGame)
	}
}

func TestSearchSurfacesLiveGameWithClock(t *testing.T) {
	games := []map[string]any{
		game(gameOpts{id: 1, date: "2026-10-20", start: "2026-10-20T23:00:00Z", state: "OFF",
			away: team("TOR", "Toronto Maple Leafs", 6), home: team("CBJ", "Columbus Blue Jackets", 2),
			venue: "Nationwide Arena", period: 3, periodType: "REG"}),
		game(gameOpts{id: 2026020155, date: "2026-10-22", start: "2026-10-22T23:00:00Z", state: "LIVE",
			away: team("NYR", "New York Rangers", 1), home: team("TOR", "Toronto Maple Leafs", 3),
			venue: "Scotiabank Arena", period: 2, periodType: "REG"}),
		game(gameOpts{id: 3, date: "2026-10-24", start: "2026-10-24T23:00:00Z", state: "FUT",
			away: futTeam("TOR", "Toronto Maple Leafs"), home: futTeam("EDM", "Edmonton Oilers"),
			venue: "Rogers Place"}),
	}
	stub := newStubNHL(t, games)
	defer stub.Close()
	// The landing carries scores too, but the scoreboard is the one source
	// of truth for the score; the landing only contributes clock and period.
	stub.SetLanding("2026020155", landing("LIVE", 2, "REG", "12:34", false, 9, 9))

	snap, err := hockey.Search(context.Background(), stub.Client(), stub.URL(), leafsConfig(), rfcTime(t, "2026-10-23T00:10:00Z"))
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if snap.LiveGame == nil {
		t.Fatal("expected LiveGame")
	}
	live := snap.LiveGame
	if !live.IsLive || live.IsFinal {
		t.Errorf("live flags: IsLive=%v IsFinal=%v", live.IsLive, live.IsFinal)
	}
	if live.TeamScore != 3 || live.OpponentScore != 1 {
		t.Errorf("score = %d-%d, want 3-1", live.TeamScore, live.OpponentScore)
	}
	if live.Opponent != "New York Rangers" || live.HomeAway != "home" {
		t.Errorf("Opponent=%q HomeAway=%q", live.Opponent, live.HomeAway)
	}
	if live.Period != 2 || live.PeriodType != "REG" {
		t.Errorf("Period=%d PeriodType=%q, want 2 REG", live.Period, live.PeriodType)
	}
	if live.Clock != "12:34" {
		t.Errorf("Clock = %q, want 12:34", live.Clock)
	}
	if live.InIntermission {
		t.Errorf("InIntermission = true, want false")
	}
	// The live game must not also be reported as latest or next.
	if snap.LatestGame == nil || snap.LatestGame.Opponent != "Columbus Blue Jackets" {
		t.Errorf("LatestGame = %+v, want Columbus", snap.LatestGame)
	}
	if snap.NextGame == nil || snap.NextGame.Opponent != "Edmonton Oilers" {
		t.Errorf("NextGame = %+v, want Edmonton", snap.NextGame)
	}
}

func TestSearchReportsIntermission(t *testing.T) {
	games := []map[string]any{
		game(gameOpts{id: 2026020155, date: "2026-10-22", start: "2026-10-22T23:00:00Z", state: "LIVE",
			away: team("NYR", "New York Rangers", 1), home: team("TOR", "Toronto Maple Leafs", 3),
			venue: "Scotiabank Arena", period: 1, periodType: "REG"}),
	}
	stub := newStubNHL(t, games)
	defer stub.Close()
	stub.SetLanding("2026020155", landing("LIVE", 1, "REG", "00:00", true, 1, 3))

	snap, err := hockey.Search(context.Background(), stub.Client(), stub.URL(), leafsConfig(), rfcTime(t, "2026-10-22T23:50:00Z"))
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if snap.LiveGame == nil {
		t.Fatal("expected LiveGame")
	}
	if !snap.LiveGame.InIntermission {
		t.Errorf("InIntermission = false, want true")
	}
	if snap.LiveGame.Period != 1 {
		t.Errorf("Period = %d, want 1", snap.LiveGame.Period)
	}
}

func TestSearchTreatsCritAsLive(t *testing.T) {
	games := []map[string]any{
		game(gameOpts{id: 2026020155, date: "2026-10-22", start: "2026-10-22T23:00:00Z", state: "CRIT",
			away: team("NYR", "New York Rangers", 2), home: team("TOR", "Toronto Maple Leafs", 2),
			venue: "Scotiabank Arena", period: 3, periodType: "REG"}),
	}
	stub := newStubNHL(t, games)
	defer stub.Close()
	stub.SetLanding("2026020155", landing("CRIT", 3, "REG", "01:15", false, 2, 2))

	snap, err := hockey.Search(context.Background(), stub.Client(), stub.URL(), leafsConfig(), rfcTime(t, "2026-10-23T01:30:00Z"))
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if snap.LiveGame == nil || !snap.LiveGame.IsLive {
		t.Fatalf("expected CRIT game as LiveGame, got %+v", snap.LiveGame)
	}
	if snap.LiveGame.Clock != "01:15" {
		t.Errorf("Clock = %q, want 01:15", snap.LiveGame.Clock)
	}
	if snap.LatestGame != nil {
		t.Errorf("CRIT game must not also be LatestGame: %+v", snap.LatestGame)
	}
}

func TestSearchLiveGameSurvivesLandingFailure(t *testing.T) {
	games := []map[string]any{
		game(gameOpts{id: 2026020155, date: "2026-10-22", start: "2026-10-22T23:00:00Z", state: "LIVE",
			away: team("NYR", "New York Rangers", 1), home: team("TOR", "Toronto Maple Leafs", 3),
			venue: "Scotiabank Arena", period: 2, periodType: "REG"}),
	}
	stub := newStubNHL(t, games)
	defer stub.Close()
	stub.FailLandings()

	snap, err := hockey.Search(context.Background(), stub.Client(), stub.URL(), leafsConfig(), rfcTime(t, "2026-10-23T00:10:00Z"))
	if err != nil {
		t.Fatalf("Search should not fail when only the landing call fails: %v", err)
	}
	if snap.LiveGame == nil {
		t.Fatal("expected LiveGame even without landing detail")
	}
	if snap.LiveGame.TeamScore != 3 || snap.LiveGame.OpponentScore != 1 {
		t.Errorf("score = %d-%d, want 3-1 from scoreboard", snap.LiveGame.TeamScore, snap.LiveGame.OpponentScore)
	}
	if snap.LiveGame.Period != 2 {
		t.Errorf("Period = %d, want 2 from scoreboard", snap.LiveGame.Period)
	}
	if snap.LiveGame.Clock != "" {
		t.Errorf("Clock = %q, want empty when landing unavailable", snap.LiveGame.Clock)
	}
}

func TestSearchDoesNotFetchLandingWithoutLiveGame(t *testing.T) {
	games := []map[string]any{
		game(gameOpts{id: 1, date: "2026-10-20", start: "2026-10-20T23:00:00Z", state: "OFF",
			away: team("TOR", "Toronto Maple Leafs", 6), home: team("CBJ", "Columbus Blue Jackets", 2),
			venue: "Nationwide Arena", period: 3, periodType: "REG"}),
	}
	stub := newStubNHL(t, games)
	defer stub.Close()

	if _, err := hockey.Search(context.Background(), stub.Client(), stub.URL(), leafsConfig(), rfcTime(t, "2026-10-21T12:00:00Z")); err != nil {
		t.Fatalf("Search: %v", err)
	}
	for _, p := range stub.Paths() {
		if strings.HasPrefix(p, "/v1/gamecenter/") {
			t.Errorf("landing endpoint hit with no live game: %s", p)
		}
	}
}

func TestSearchReturnsNilGamesWhenScoreboardEmpty(t *testing.T) {
	stub := newStubNHL(t, nil)
	defer stub.Close()

	snap, err := hockey.Search(context.Background(), stub.Client(), stub.URL(), leafsConfig(), rfcTime(t, "2026-07-01T12:00:00Z"))
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if snap == nil {
		t.Fatal("expected non-nil snapshot")
	}
	if snap.LiveGame != nil || snap.LatestGame != nil || snap.NextGame != nil {
		t.Errorf("expected all games nil, got live=%+v latest=%+v next=%+v", snap.LiveGame, snap.LatestGame, snap.NextGame)
	}
}

func TestSearchIgnoresGamesNotInvolvingTeam(t *testing.T) {
	games := []map[string]any{
		game(gameOpts{id: 1, date: "2026-10-20", start: "2026-10-20T23:00:00Z", state: "OFF",
			away: team("BOS", "Boston Bruins", 6), home: team("CBJ", "Columbus Blue Jackets", 2),
			venue: "Nationwide Arena", period: 3, periodType: "REG"}),
	}
	stub := newStubNHL(t, games)
	defer stub.Close()

	snap, err := hockey.Search(context.Background(), stub.Client(), stub.URL(), leafsConfig(), rfcTime(t, "2026-10-21T12:00:00Z"))
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if snap.LatestGame != nil {
		t.Errorf("expected no LatestGame for a game without TOR, got %+v", snap.LatestGame)
	}
}

func TestSearchReturnsErrorOnUpstreamFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := hockey.Search(context.Background(), srv.Client(), srv.URL, leafsConfig(), rfcTime(t, "2026-10-21T12:00:00Z"))
	if err == nil {
		t.Fatal("expected error on upstream 500")
	}
}

func TestSearchRequestsTeamScoreboard(t *testing.T) {
	stub := newStubNHL(t, nil)
	defer stub.Close()

	if _, err := hockey.Search(context.Background(), stub.Client(), stub.URL(), leafsConfig(), rfcTime(t, "2026-10-21T12:00:00Z")); err != nil {
		t.Fatalf("Search: %v", err)
	}
	paths := stub.Paths()
	found := false
	for _, p := range paths {
		if p == "/v1/scoreboard/TOR/now" {
			found = true
		}
	}
	if !found {
		t.Errorf("paths = %v, want /v1/scoreboard/TOR/now", paths)
	}
}

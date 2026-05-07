package baseball_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jpatters/home-calendar/internal/baseball"
)

// fakeMLB is a stub that always returns the configured response and counts how
// many times the schedule endpoint is hit.
type fakeMLB struct {
	srv      *httptest.Server
	hits     atomic.Int32
	response func() []stubDate
}

func newFakeMLB(t *testing.T, response func() []stubDate) *fakeMLB {
	t.Helper()
	f := &fakeMLB{response: response}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"totalGames": 0,
			"dates":      f.response(),
		})
	}))
	return f
}

func (f *fakeMLB) Hits() int32 { return f.hits.Load() }
func (f *fakeMLB) URL() string { return f.srv.URL }
func (f *fakeMLB) Close()      { f.srv.Close() }

func TestFetcherUsesLiveIntervalWhenLiveGamePresent(t *testing.T) {
	// When the team has a live game, the fetcher should poll on the (faster)
	// live interval, not the normal interval.
	live := []stubDate{{
		Date: "2026-04-22",
		Games: []stubGame{{
			GamePk: 1, GameDate: "2026-04-22T23:00:00Z", GameType: "R",
			Status: stubStatus{AbstractGameState: "Live", DetailedState: "In Progress"},
			Teams: stubTeams{
				Home: stubSide{Team: stubTeam{ID: yankeesID, Name: "New York Yankees", Abbreviation: "NYY"}, Score: 1},
				Away: stubSide{Team: stubTeam{ID: 111, Name: "Boston Red Sox", Abbreviation: "BOS"}, Score: 0},
			},
			Linescore: stubLinescore{CurrentInning: 2, InningHalf: "Top", Outs: 1},
		}},
	}}
	mlb := newFakeMLB(t, func() []stubDate { return live })
	defer mlb.Close()

	f := baseball.New(mlb.URL(), nil)
	defer f.Stop()
	// Normal interval long enough that we'd never see two hits if it were used.
	// Live interval is 100ms so we should see several hits in ~500ms.
	f.Start(context.Background(), yankeesConfig(), 1*time.Hour, 100*time.Millisecond)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if mlb.Hits() >= 3 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got := mlb.Hits(); got < 3 {
		t.Errorf("expected fetcher to poll fast while live (>=3 hits in 2s), got %d", got)
	}
}

func TestFetcherDropsBackToNormalIntervalAfterGameEnds(t *testing.T) {
	// When a game flips from Live to Final, subsequent fetches should occur on
	// the normal cadence rather than the fast live cadence.
	finalGames := []stubDate{{
		Date: "2026-04-22",
		Games: []stubGame{{
			GamePk: 1, GameDate: "2026-04-22T23:00:00Z", GameType: "R",
			Status: stubStatus{AbstractGameState: "Final", DetailedState: "Final"},
			Teams: stubTeams{
				Home: stubSide{Team: stubTeam{ID: yankeesID, Name: "New York Yankees", Abbreviation: "NYY"}, Score: 4},
				Away: stubSide{Team: stubTeam{ID: 111, Name: "Boston Red Sox", Abbreviation: "BOS"}, Score: 1},
			},
		}},
	}}
	liveGames := []stubDate{{
		Date: "2026-04-22",
		Games: []stubGame{{
			GamePk: 1, GameDate: "2026-04-22T23:00:00Z", GameType: "R",
			Status: stubStatus{AbstractGameState: "Live", DetailedState: "In Progress"},
			Teams: stubTeams{
				Home: stubSide{Team: stubTeam{ID: yankeesID, Name: "New York Yankees", Abbreviation: "NYY"}, Score: 2},
				Away: stubSide{Team: stubTeam{ID: 111, Name: "Boston Red Sox", Abbreviation: "BOS"}, Score: 0},
			},
			Linescore: stubLinescore{CurrentInning: 4, InningHalf: "Top", Outs: 2},
		}},
	}}
	state := atomic.Pointer[[]stubDate]{}
	state.Store(&liveGames)
	mlb := newFakeMLB(t, func() []stubDate {
		return *state.Load()
	})
	defer mlb.Close()

	f := baseball.New(mlb.URL(), nil)
	defer f.Stop()
	// Normal=200ms, live=50ms. After flipping to Final, we should see the
	// inter-poll spacing widen.
	f.Start(context.Background(), yankeesConfig(), 200*time.Millisecond, 50*time.Millisecond)

	// Wait for ~3 live polls then flip to Final.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if mlb.Hits() >= 3 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	hitsAtFlip := mlb.Hits()
	state.Store(&finalGames)

	// Wait for one more poll to land (cadence still live, but next snapshot is
	// Final, so the ONE AFTER that should use normal cadence).
	deadline = time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if mlb.Hits() >= hitsAtFlip+1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	hitsAfterFlip := mlb.Hits()
	if hitsAfterFlip < hitsAtFlip+1 {
		t.Fatalf("expected at least one poll after flip; flip=%d after=%d", hitsAtFlip, hitsAfterFlip)
	}

	// Now if we're on the normal interval (200ms), in the next 250ms we should
	// see at most one more hit. If we were still on live cadence (50ms), we'd
	// see roughly 5.
	t0 := time.Now()
	hitsT0 := mlb.Hits()
	time.Sleep(250 * time.Millisecond)
	delta := mlb.Hits() - hitsT0
	if delta > 2 {
		t.Errorf("after game ended, fetcher still polling fast: %d hits in %v starting at %v",
			delta, time.Since(t0), t0)
	}
}

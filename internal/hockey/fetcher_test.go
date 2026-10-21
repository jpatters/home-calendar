package hockey_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jpatters/home-calendar/internal/hockey"
)

func liveLeafsGame() []map[string]any {
	return []map[string]any{
		game(gameOpts{id: 2026020155, date: "2026-10-22", start: "2026-10-22T23:00:00Z", state: "LIVE",
			away: team("NYR", "New York Rangers", 1), home: team("TOR", "Toronto Maple Leafs", 3),
			venue: "Scotiabank Arena", period: 2, periodType: "REG"}),
	}
}

func finishedLeafsGame() []map[string]any {
	return []map[string]any{
		game(gameOpts{id: 2026020155, date: "2026-10-22", start: "2026-10-22T23:00:00Z", state: "OFF",
			away: team("NYR", "New York Rangers", 1), home: team("TOR", "Toronto Maple Leafs", 4),
			venue: "Scotiabank Arena", period: 3, periodType: "REG"}),
	}
}

func scoreboardHits(s *stubNHL) int {
	n := 0
	for _, p := range s.Paths() {
		if strings.HasPrefix(p, "/v1/scoreboard/") {
			n++
		}
	}
	return n
}

func TestFetcherUsesLiveIntervalWhenLiveGamePresent(t *testing.T) {
	stub := newStubNHL(t, liveLeafsGame())
	defer stub.Close()
	stub.SetLanding("2026020155", landing("LIVE", 2, "REG", "12:34", false, 1, 3))

	f := hockey.New(stub.URL(), nil)
	defer f.Stop()
	// Normal interval long enough that we'd never see two hits if it were
	// used; the live interval should produce several hits quickly.
	f.Start(context.Background(), leafsConfig(), 1*time.Hour, 100*time.Millisecond)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if scoreboardHits(stub) >= 3 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got := scoreboardHits(stub); got < 3 {
		t.Fatalf("expected >= 3 scoreboard polls on live cadence, got %d", got)
	}
	snap := f.Snapshot()
	if snap == nil || snap.LiveGame == nil || snap.LiveGame.Clock != "12:34" {
		t.Errorf("snapshot missing live clock: %+v", snap)
	}
}

func TestFetcherDropsBackToNormalIntervalAfterGameEnds(t *testing.T) {
	stub := newStubNHL(t, liveLeafsGame())
	defer stub.Close()
	stub.SetLanding("2026020155", landing("LIVE", 2, "REG", "12:34", false, 1, 3))

	f := hockey.New(stub.URL(), nil)
	defer f.Stop()
	f.Start(context.Background(), leafsConfig(), 200*time.Millisecond, 50*time.Millisecond)

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if scoreboardHits(stub) >= 3 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	hitsAtFlip := scoreboardHits(stub)

	stub.mu.Lock()
	stub.games = finishedLeafsGame()
	stub.mu.Unlock()

	// One more poll lands on the live cadence; the one after it should use
	// the normal cadence because the snapshot no longer has a live game.
	deadline = time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if scoreboardHits(stub) >= hitsAtFlip+1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if scoreboardHits(stub) < hitsAtFlip+1 {
		t.Fatalf("expected at least one poll after flip")
	}

	hitsT0 := scoreboardHits(stub)
	time.Sleep(250 * time.Millisecond)
	delta := scoreboardHits(stub) - hitsT0
	if delta > 2 {
		t.Errorf("after game ended, fetcher still polling fast: %d hits in 250ms", delta)
	}
}

func TestFetcherStopClearsSnapshot(t *testing.T) {
	stub := newStubNHL(t, finishedLeafsGame())
	defer stub.Close()

	f := hockey.New(stub.URL(), nil)
	f.Start(context.Background(), leafsConfig(), time.Hour, time.Hour)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && f.Snapshot() == nil {
		time.Sleep(10 * time.Millisecond)
	}
	if f.Snapshot() == nil {
		t.Fatal("expected a snapshot after start")
	}
	f.Stop()
	if f.Snapshot() != nil {
		t.Errorf("expected nil snapshot after Stop")
	}
}

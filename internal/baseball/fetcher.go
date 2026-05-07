package baseball

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/jpatters/home-calendar/internal/types"
)

const defaultLiveInterval = 30 * time.Second

type Fetcher struct {
	client      *http.Client
	scheduleURL string

	mu       sync.RWMutex
	snapshot *types.BaseballSnapshot
	lastErr  error

	cancel   context.CancelFunc
	doneWG   sync.WaitGroup
	onUpdate func(*types.BaseballSnapshot)
}

// New constructs a Fetcher that polls scheduleURL. Pass DefaultScheduleURL in
// production; tests pass an httptest server URL.
func New(scheduleURL string, onUpdate func(*types.BaseballSnapshot)) *Fetcher {
	return &Fetcher{
		client:      &http.Client{Timeout: 20 * time.Second},
		scheduleURL: scheduleURL,
		onUpdate:    onUpdate,
	}
}

func (f *Fetcher) HTTPClient() *http.Client {
	return f.client
}

func (f *Fetcher) Snapshot() *types.BaseballSnapshot {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.snapshot == nil {
		return nil
	}
	s := *f.snapshot
	if f.snapshot.LiveGame != nil {
		g := *f.snapshot.LiveGame
		s.LiveGame = &g
	}
	if f.snapshot.LatestGame != nil {
		g := *f.snapshot.LatestGame
		s.LatestGame = &g
	}
	if f.snapshot.NextGame != nil {
		g := *f.snapshot.NextGame
		s.NextGame = &g
	}
	return &s
}

// Start begins polling the schedule API. The fetcher uses normalInterval for
// routine polling and liveInterval (faster) while the team has a live game.
func (f *Fetcher) Start(parent context.Context, b types.Baseball, normalInterval, liveInterval time.Duration) {
	f.Stop()
	if b.TeamID == 0 {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	f.cancel = cancel
	f.doneWG.Add(1)
	go f.loop(ctx, b, normalInterval, liveInterval)
}

func (f *Fetcher) Stop() {
	if f.cancel != nil {
		f.cancel()
		f.doneWG.Wait()
		f.cancel = nil
	}
	f.mu.Lock()
	f.snapshot = nil
	f.lastErr = nil
	f.mu.Unlock()
}

func (f *Fetcher) RefreshNow(ctx context.Context, b types.Baseball) {
	f.fetch(ctx, b)
}

func (f *Fetcher) loop(ctx context.Context, b types.Baseball, normalInterval, liveInterval time.Duration) {
	defer f.doneWG.Done()
	if normalInterval <= 0 {
		normalInterval = 10 * time.Minute
	}
	if liveInterval <= 0 {
		liveInterval = defaultLiveInterval
	}
	f.fetch(ctx, b)
	timer := time.NewTimer(f.nextInterval(normalInterval, liveInterval))
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			f.fetch(ctx, b)
			timer.Reset(f.nextInterval(normalInterval, liveInterval))
		}
	}
}

// nextInterval returns the live interval when the most recent snapshot has a
// live game, otherwise the normal interval.
func (f *Fetcher) nextInterval(normalInterval, liveInterval time.Duration) time.Duration {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.snapshot != nil && f.snapshot.LiveGame != nil {
		return liveInterval
	}
	return normalInterval
}

func (f *Fetcher) fetch(ctx context.Context, b types.Baseball) {
	if b.TeamID == 0 {
		f.mu.Lock()
		hadSnapshot := f.snapshot != nil
		f.snapshot = nil
		f.mu.Unlock()
		if hadSnapshot && f.onUpdate != nil {
			f.onUpdate(nil)
		}
		return
	}
	snap, err := Search(ctx, f.client, f.scheduleURL, b, time.Now())
	if err != nil {
		log.Printf("baseball: %v", err)
		f.mu.Lock()
		f.lastErr = err
		f.mu.Unlock()
		return
	}
	f.mu.Lock()
	f.snapshot = snap
	f.lastErr = nil
	f.mu.Unlock()
	if f.onUpdate != nil {
		f.onUpdate(snap)
	}
}

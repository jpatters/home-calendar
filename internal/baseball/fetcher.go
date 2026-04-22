package baseball

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/jpatters/home-calendar/internal/types"
)

type Fetcher struct {
	client *http.Client

	mu       sync.RWMutex
	snapshot *types.BaseballSnapshot
	lastErr  error

	cancel   context.CancelFunc
	doneWG   sync.WaitGroup
	onUpdate func(*types.BaseballSnapshot)
}

func New(onUpdate func(*types.BaseballSnapshot)) *Fetcher {
	return &Fetcher{
		client:   &http.Client{Timeout: 20 * time.Second},
		onUpdate: onUpdate,
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

func (f *Fetcher) Start(parent context.Context, b types.Baseball, interval time.Duration) {
	f.Stop()
	if b.TeamID == 0 {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	f.cancel = cancel
	f.doneWG.Add(1)
	go f.loop(ctx, b, interval)
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

func (f *Fetcher) loop(ctx context.Context, b types.Baseball, interval time.Duration) {
	defer f.doneWG.Done()
	f.fetch(ctx, b)
	if interval <= 0 {
		interval = 10 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			f.fetch(ctx, b)
		}
	}
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
	snap, err := Search(ctx, f.client, DefaultScheduleURL, b, time.Now())
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

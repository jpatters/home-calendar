package snowday

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/jpatters/home-calendar/internal/types"
)

const (
	userAgent       = "home-calendar/1.0 (+https://github.com/jpatters/home-calendar)"
	defaultInterval = 30 * time.Minute
)

type Fetcher struct {
	client *http.Client

	mu       sync.RWMutex
	snapshot *types.SnowDaySnapshot
	lastErr  error

	lifecycleMu sync.Mutex
	cancel      context.CancelFunc
	doneWG      sync.WaitGroup
	onUpdate    func(*types.SnowDaySnapshot)
}

func New(onUpdate func(*types.SnowDaySnapshot)) *Fetcher {
	return &Fetcher{
		client:   &http.Client{Timeout: 20 * time.Second},
		onUpdate: onUpdate,
	}
}

func (f *Fetcher) Snapshot() *types.SnowDaySnapshot {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.snapshot == nil {
		return nil
	}
	s := *f.snapshot
	return &s
}

func (f *Fetcher) Start(parent context.Context, s types.SnowDay, _ time.Duration) {
	f.Stop()
	f.lifecycleMu.Lock()
	defer f.lifecycleMu.Unlock()
	ctx, cancel := context.WithCancel(parent)
	f.cancel = cancel
	f.doneWG.Add(1)
	go f.loop(ctx, s)
}

func (f *Fetcher) Stop() {
	f.lifecycleMu.Lock()
	cancel := f.cancel
	f.cancel = nil
	f.lifecycleMu.Unlock()
	if cancel != nil {
		cancel()
		f.doneWG.Wait()
	}
	f.mu.Lock()
	f.snapshot = nil
	f.lastErr = nil
	f.mu.Unlock()
}

func (f *Fetcher) RefreshNow(ctx context.Context, s types.SnowDay) {
	f.fetch(ctx, s)
}

func (f *Fetcher) loop(ctx context.Context, s types.SnowDay) {
	defer f.doneWG.Done()
	f.fetch(ctx, s)
	t := time.NewTicker(defaultInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			f.fetch(ctx, s)
		}
	}
}

type apiResponse struct {
	Prediction *struct {
		Score       int    `json:"score"`
		Category    string `json:"category"`
		Probability int    `json:"probability"`
		Debug       struct {
			NextMorning string `json:"nextMorning"`
		} `json:"debug"`
	} `json:"prediction"`
	City       string `json:"city"`
	State      string `json:"state"`
	RegionName string `json:"region_name"`
}

func (f *Fetcher) fetch(ctx context.Context, s types.SnowDay) {
	if strings.TrimSpace(s.URL) == "" {
		return
	}
	apiURL, err := toAPIURL(s.URL)
	if err != nil {
		f.setErr(err)
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		f.setErr(err)
		return
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := f.client.Do(req)
	if err != nil {
		f.setErr(err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		f.setErr(fmt.Errorf("snowday http %d", resp.StatusCode))
		return
	}
	var body apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		f.setErr(err)
		return
	}
	if body.Prediction == nil {
		f.setErr(fmt.Errorf("snowday: response missing prediction"))
		return
	}

	snap := &types.SnowDaySnapshot{
		UpdatedAt:   time.Now(),
		URL:         s.URL,
		Location:    buildLocation(body.City, body.State),
		RegionName:  body.RegionName,
		Probability: body.Prediction.Probability,
		Score:       body.Prediction.Score,
		Category:    body.Prediction.Category,
	}
	if t, err := time.Parse(time.RFC3339, body.Prediction.Debug.NextMorning); err == nil {
		snap.MorningTime = t
	}

	f.mu.Lock()
	f.snapshot = snap
	f.lastErr = nil
	f.mu.Unlock()
	if f.onUpdate != nil {
		f.onUpdate(snap)
	}
}

func (f *Fetcher) setErr(err error) {
	log.Printf("snowday: %v", err)
	f.mu.Lock()
	f.lastErr = err
	f.mu.Unlock()
}

func buildLocation(city, state string) string {
	switch {
	case city != "" && state != "":
		return city + ", " + state
	case city != "":
		return city
	default:
		return state
	}
}

// toAPIURL accepts either the public prediction page URL or the API URL and
// always returns the API URL. Paths containing "/prediction/" are rewritten
// to "/api/query/"; query strings and fragments are dropped since the API
// does not use them. Only http(s) schemes are accepted.
func toAPIURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("snowday: unsupported URL scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return "", fmt.Errorf("snowday: URL missing host")
	}
	if strings.Contains(u.Path, "/prediction/") {
		u.Path = strings.Replace(u.Path, "/prediction/", "/api/query/", 1)
	}
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

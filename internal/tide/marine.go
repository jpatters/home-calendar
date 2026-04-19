package tide

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/jpatters/home-calendar/internal/types"
)

const DefaultMarineURL = "https://marine-api.open-meteo.com/v1/marine"

type Fetcher struct {
	client *http.Client

	mu       sync.RWMutex
	snapshot *types.TideSnapshot
	lastErr  error

	cancel   context.CancelFunc
	doneWG   sync.WaitGroup
	onUpdate func(*types.TideSnapshot)
}

func New(onUpdate func(*types.TideSnapshot)) *Fetcher {
	return &Fetcher{
		client:   &http.Client{Timeout: 20 * time.Second},
		onUpdate: onUpdate,
	}
}

func (f *Fetcher) Snapshot() *types.TideSnapshot {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.snapshot == nil {
		return nil
	}
	s := *f.snapshot
	s.Events = append([]types.TideEvent(nil), f.snapshot.Events...)
	return &s
}

func (f *Fetcher) Start(parent context.Context, t types.Tide, interval time.Duration) {
	f.Stop()
	ctx, cancel := context.WithCancel(parent)
	f.cancel = cancel
	f.doneWG.Add(1)
	go f.loop(ctx, t, interval)
}

func (f *Fetcher) Stop() {
	if f.cancel != nil {
		f.cancel()
		f.doneWG.Wait()
		f.cancel = nil
	}
}

func (f *Fetcher) RefreshNow(ctx context.Context, t types.Tide) {
	f.fetch(ctx, t)
}

func (f *Fetcher) loop(ctx context.Context, t types.Tide, interval time.Duration) {
	defer f.doneWG.Done()
	f.fetch(ctx, t)
	if interval <= 0 {
		interval = time.Hour
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			f.fetch(ctx, t)
		}
	}
}

func (f *Fetcher) fetch(ctx context.Context, t types.Tide) {
	if t.Latitude == 0 && t.Longitude == 0 {
		f.mu.Lock()
		hadSnapshot := f.snapshot != nil
		f.snapshot = nil
		f.mu.Unlock()
		if hadSnapshot && f.onUpdate != nil {
			f.onUpdate(nil)
		}
		return
	}
	snap, err := Search(ctx, f.client, DefaultMarineURL, t, time.Now())
	if err != nil {
		log.Printf("tide: %v", err)
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

type marineResponse struct {
	Timezone string `json:"timezone"`
	Hourly   struct {
		Time              []string  `json:"time"`
		SeaLevelHeightMSL []float64 `json:"sea_level_height_msl"`
	} `json:"hourly"`
}

// Search fetches hourly sea-level heights for a location from an Open-Meteo
// Marine-compatible endpoint and returns a snapshot containing the current
// interpolated height plus upcoming high/low tide events (filtered to those
// at or after `now`).
func Search(ctx context.Context, client *http.Client, baseURL string, t types.Tide, now time.Time) (*types.TideSnapshot, error) {
	if client == nil {
		client = http.DefaultClient
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("tide: parse URL: %w", err)
	}
	q := u.Query()
	q.Set("latitude", strconv.FormatFloat(t.Latitude, 'f', 4, 64))
	q.Set("longitude", strconv.FormatFloat(t.Longitude, 'f', 4, 64))
	q.Set("hourly", "sea_level_height_msl")
	q.Set("forecast_days", "8")
	// Always request UTC timestamps: we parse them as UTC on the server,
	// serialize with a Z suffix, and let the browser convert to local time
	// for display. Requesting timezone=auto would return naive local
	// strings that Go would mis-interpret as UTC.
	q.Set("timezone", "UTC")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tide: open-meteo http %d", resp.StatusCode)
	}
	var body marineResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("tide: decode response: %w", err)
	}

	times, heights := parseHourly(body.Hourly.Time, body.Hourly.SeaLevelHeightMSL)
	snap := &types.TideSnapshot{
		UpdatedAt:     time.Now(),
		Units:         defaultString(t.Units, "metric"),
		Timezone:      body.Timezone,
		CurrentMeters: currentHeight(times, heights, now),
		Events:        detectEvents(times, heights, now),
	}
	return snap, nil
}

func parseHourly(rawTimes []string, rawHeights []float64) ([]time.Time, []float64) {
	n := len(rawTimes)
	if len(rawHeights) < n {
		n = len(rawHeights)
	}
	times := make([]time.Time, 0, n)
	heights := make([]float64, 0, n)
	for i := 0; i < n; i++ {
		ts, err := time.Parse("2006-01-02T15:04", rawTimes[i])
		if err != nil {
			continue
		}
		times = append(times, ts)
		heights = append(heights, rawHeights[i])
	}
	return times, heights
}

// detectEvents scans hourly heights for local maxima ("high") and minima
// ("low"). A run of equal consecutive heights (plateau) produces a single
// event at the run midpoint. Events with a time earlier than `now` are
// dropped.
func detectEvents(times []time.Time, heights []float64, now time.Time) []types.TideEvent {
	events := []types.TideEvent{}
	if len(heights) < 3 {
		return events
	}
	i := 1
	for i < len(heights)-1 {
		// Extend j through any equal-valued plateau starting at i.
		j := i
		for j < len(heights)-1 && heights[j+1] == heights[i] {
			j++
		}
		// Plateau runs to the last sample → no right neighbour to compare.
		if j >= len(heights)-1 {
			break
		}
		left := heights[i-1]
		right := heights[j+1]
		mid := heights[i]
		if mid > left && mid > right {
			events = append(events, types.TideEvent{
				Time:         midpoint(times[i], times[j]),
				Type:         "high",
				HeightMeters: mid,
			})
		} else if mid < left && mid < right {
			events = append(events, types.TideEvent{
				Time:         midpoint(times[i], times[j]),
				Type:         "low",
				HeightMeters: mid,
			})
		}
		i = j + 1
	}
	filtered := events[:0]
	for _, ev := range events {
		if !ev.Time.Before(now) {
			filtered = append(filtered, ev)
		}
	}
	return filtered
}

func midpoint(a, b time.Time) time.Time {
	if a.Equal(b) {
		return a
	}
	return a.Add(b.Sub(a) / 2)
}

// currentHeight linearly interpolates between the two hourly samples
// bracketing `now`. If `now` is outside the sample range, returns the nearest
// edge sample (or 0 when there are no samples at all).
func currentHeight(times []time.Time, heights []float64, now time.Time) float64 {
	if len(times) == 0 {
		return 0
	}
	if now.Before(times[0]) {
		return heights[0]
	}
	for i := 0; i < len(times)-1; i++ {
		if !now.Before(times[i]) && now.Before(times[i+1]) {
			span := times[i+1].Sub(times[i])
			if span <= 0 {
				return heights[i]
			}
			frac := float64(now.Sub(times[i])) / float64(span)
			return heights[i] + (heights[i+1]-heights[i])*frac
		}
	}
	return heights[len(heights)-1]
}

func defaultString(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

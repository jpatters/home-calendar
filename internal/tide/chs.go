package tide

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/jpatters/home-calendar/internal/types"
)

// DefaultBaseURL is the CHS IWLS service operated by Fisheries and Oceans
// Canada. Predictions are referenced to chart datum, matching published
// Canadian tide tables. No API key is required.
const DefaultBaseURL = "https://api-iwls.dfo-mpo.gc.ca/api/v1"

const (
	// seriesHiLo carries discrete high/low turning points, unlabelled.
	seriesHiLo = "wlp-hilo"
	// seriesPredictions carries a continuous predicted water level.
	seriesPredictions = "wlp"

	// horizon is how far ahead events are reported. classifyPad extends the
	// query either side of it so the first and last events in the horizon each
	// have a neighbour to be classified against.
	horizon     = 8 * 24 * time.Hour
	classifyPad = 24 * time.Hour

	// currentWindow brackets "now" when sampling the continuous series.
	currentWindow = 30 * time.Minute
)

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

func (f *Fetcher) HTTPClient() *http.Client { return f.client }

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
	f.mu.Lock()
	f.snapshot = nil
	f.lastErr = nil
	f.mu.Unlock()
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
	if t.StationCode == "" {
		f.mu.Lock()
		hadSnapshot := f.snapshot != nil
		f.snapshot = nil
		f.mu.Unlock()
		if hadSnapshot && f.onUpdate != nil {
			f.onUpdate(nil)
		}
		return
	}
	snap, err := Search(ctx, f.client, DefaultBaseURL, t, time.Now())
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

// dataPoint is one entry of an IWLS time series. qcFlagCode is a string enum
// ("1" good, "2" not evaluated, "3" questionable), not a number.
type dataPoint struct {
	EventDate time.Time `json:"eventDate"`
	Value     float64   `json:"value"`
}

type station struct {
	ID           string `json:"id"`
	Code         string `json:"code"`
	OfficialName string `json:"officialName"`
}

// Search fetches tide predictions for the configured CHS station and returns a
// snapshot holding the present water level plus the high and low tides due
// within the reporting horizon. Heights are metres above chart datum.
func Search(ctx context.Context, client *http.Client, baseURL string, t types.Tide, now time.Time) (*types.TideSnapshot, error) {
	if client == nil {
		client = http.DefaultClient
	}
	st, err := resolveStation(ctx, client, baseURL, t.StationCode)
	if err != nil {
		return nil, err
	}

	// Query wider than the horizon: a turning point is identified by
	// comparing it with the points either side, so the events at both ends of
	// the horizon need neighbours beyond it.
	extrema, err := fetchSeries(ctx, client, baseURL, st.ID, seriesHiLo, url.Values{},
		now.Add(-classifyPad), now.Add(horizon+classifyPad))
	if err != nil {
		return nil, err
	}

	levels, err := fetchSeries(ctx, client, baseURL, st.ID, seriesPredictions,
		url.Values{"resolution": []string{"FIFTEEN_MINUTES"}},
		now.Add(-currentWindow), now.Add(currentWindow))
	if err != nil {
		return nil, err
	}
	if len(levels) == 0 {
		return nil, fmt.Errorf("tide: station %s has no predicted water level for %s", t.StationCode, now.Format(time.RFC3339))
	}

	return &types.TideSnapshot{
		UpdatedAt:     time.Now(),
		Units:         defaultString(t.Units, "metric"),
		Timezone:      t.Timezone,
		CurrentMeters: currentHeight(levels, now),
		Events:        detectEvents(extrema, now, now.Add(horizon)),
	}, nil
}

// resolveStation maps a station code to the opaque station id the data
// endpoints are keyed by. Codes are stable and human-meaningful; ids are not.
func resolveStation(ctx context.Context, client *http.Client, baseURL, code string) (station, error) {
	if code == "" {
		return station{}, fmt.Errorf("tide: no station selected")
	}
	var found []station
	if err := getJSON(ctx, client, baseURL, "/stations", url.Values{"code": []string{code}}, &found); err != nil {
		return station{}, err
	}
	// An unknown code answers 200 with an empty list rather than 404.
	if len(found) == 0 {
		return station{}, fmt.Errorf("tide: no CHS station with code %q", code)
	}
	return found[0], nil
}

func fetchSeries(ctx context.Context, client *http.Client, baseURL, stationID, seriesCode string, extra url.Values, from, to time.Time) ([]dataPoint, error) {
	q := url.Values{}
	maps.Copy(q, extra)
	q.Set("time-series-code", seriesCode)
	q.Set("from", from.UTC().Format(time.RFC3339))
	q.Set("to", to.UTC().Format(time.RFC3339))

	var out []dataPoint
	if err := getJSON(ctx, client, baseURL, "/stations/"+stationID+"/data", q, &out); err != nil {
		return nil, fmt.Errorf("tide: %s series: %w", seriesCode, err)
	}
	return out, nil
}

func getJSON(ctx context.Context, client *http.Client, baseURL, path string, q url.Values, dst any) error {
	u, err := url.Parse(baseURL + path)
	if err != nil {
		return fmt.Errorf("tide: parse URL: %w", err)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("chs http %d for %s", resp.StatusCode, path)
	}
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

// detectEvents labels each turning point as a high or a low. CHS returns the
// times and heights of the turning points but does not say which is which, so
// each point is compared with its immediate neighbours. The first and last
// points have only one neighbour and are therefore dropped — callers query a
// wider window than they report so nothing visible is lost. Results are
// trimmed to [now, until].
func detectEvents(points []dataPoint, now, until time.Time) []types.TideEvent {
	events := []types.TideEvent{}
	for i := 1; i < len(points)-1; i++ {
		prev, cur, next := points[i-1].Value, points[i].Value, points[i+1].Value
		var kind string
		switch {
		case cur > prev && cur > next:
			kind = "high"
		case cur < prev && cur < next:
			kind = "low"
		default:
			continue
		}
		at := points[i].EventDate
		if at.Before(now) || at.After(until) {
			continue
		}
		events = append(events, types.TideEvent{
			Time:         at,
			Type:         kind,
			HeightMeters: cur,
		})
	}
	return events
}

// currentHeight linearly interpolates between the two samples bracketing
// `now`. Outside the sample range it returns the nearest edge sample.
func currentHeight(points []dataPoint, now time.Time) float64 {
	if len(points) == 0 {
		return 0
	}
	if now.Before(points[0].EventDate) {
		return points[0].Value
	}
	for i := 0; i < len(points)-1; i++ {
		if !now.Before(points[i].EventDate) && now.Before(points[i+1].EventDate) {
			span := points[i+1].EventDate.Sub(points[i].EventDate)
			if span <= 0 {
				return points[i].Value
			}
			frac := float64(now.Sub(points[i].EventDate)) / float64(span)
			return points[i].Value + (points[i+1].Value-points[i].Value)*frac
		}
	}
	return points[len(points)-1].Value
}

func defaultString(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

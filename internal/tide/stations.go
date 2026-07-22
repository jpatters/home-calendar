package tide

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	// listTTL is how long a fetched station listing is reused. The catalogue
	// changes on the order of years.
	listTTL = 24 * time.Hour
	// MaxStationResults bounds what a type-ahead query returns; a broad query
	// matches hundreds of the ~1,500 stations.
	MaxStationResults = 25
)

// StationResult is a tide station offered to the admin station picker. The
// station code is unique, so it doubles as the disambiguator between stations
// sharing a name.
type StationResult struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// Directory searches the CHS station catalogue by name or code. IWLS offers no
// name search of its own, so the catalogue is fetched whole and filtered here.
// It is ~830 KB, and the admin panel queries on every keystroke, so a fetched
// listing is reused until it goes stale.
type Directory struct {
	client  *http.Client
	baseURL string

	mu        sync.Mutex
	stations  []StationResult
	fetchedAt time.Time
}

func NewDirectory(client *http.Client, baseURL string) *Directory {
	if client == nil {
		client = http.DefaultClient
	}
	return &Directory{client: client, baseURL: baseURL}
}

func (d *Directory) Search(ctx context.Context, query string) ([]StationResult, error) {
	stations, err := d.list(ctx)
	if err != nil {
		return nil, err
	}
	needle := strings.ToLower(strings.TrimSpace(query))
	out := []StationResult{}
	for _, s := range stations {
		if needle != "" && !matchesStation(s, needle) {
			continue
		}
		out = append(out, s)
		if len(out) == MaxStationResults {
			break
		}
	}
	return out, nil
}

func matchesStation(s StationResult, needle string) bool {
	return strings.Contains(strings.ToLower(s.Name), needle) ||
		strings.Contains(strings.ToLower(s.Code), needle)
}

func (d *Directory) list(ctx context.Context) ([]StationResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stations != nil && time.Since(d.fetchedAt) < listTTL {
		return d.stations, nil
	}

	// The widget needs high/low events and a present water level, so only
	// stations publishing both series are usable. IWLS can filter by one
	// series code per request, so ask twice and keep the intersection.
	// Discontinued and non-operating stations are kept: many still publish
	// predictions years ahead, including Canoe Cove.
	withEvents, err := d.listWithSeries(ctx, seriesHiLo)
	if err != nil {
		return nil, err
	}
	withLevels, err := d.listWithSeries(ctx, seriesPredictions)
	if err != nil {
		return nil, err
	}
	usable := make(map[string]bool, len(withLevels))
	for _, s := range withLevels {
		usable[s.Code] = true
	}
	out := make([]StationResult, 0, len(withEvents))
	for _, s := range withEvents {
		if usable[s.Code] {
			out = append(out, s)
		}
	}

	d.stations = out
	d.fetchedAt = time.Now()
	return out, nil
}

func (d *Directory) listWithSeries(ctx context.Context, seriesCode string) ([]StationResult, error) {
	var found []station
	q := url.Values{"time-series-code": []string{seriesCode}}
	if err := getJSON(ctx, d.client, d.baseURL, "/stations", q, &found); err != nil {
		return nil, err
	}
	out := make([]StationResult, 0, len(found))
	for _, s := range found {
		out = append(out, StationResult{Code: s.Code, Name: s.OfficialName})
	}
	return out, nil
}

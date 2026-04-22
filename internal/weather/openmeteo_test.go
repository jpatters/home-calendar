package weather

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/jpatters/home-calendar/internal/types"
)

const openMeteoSampleBody = `{
  "timezone": "America/Toronto",
  "current": {
    "time": "2026-04-21T12:00",
    "temperature_2m": 14.2,
    "apparent_temperature": 13.0,
    "relative_humidity_2m": 60,
    "wind_speed_10m": 11.0,
    "weather_code": 3,
    "is_day": 1,
    "precipitation": 0.0
  },
  "daily": {
    "time": ["2026-04-21","2026-04-22","2026-04-23","2026-04-24","2026-04-25","2026-04-26","2026-04-27"],
    "temperature_2m_max": [18.0,19.0,20.0,21.0,22.0,23.0,24.0],
    "temperature_2m_min": [8.0,9.0,10.0,11.0,12.0,13.0,14.0],
    "weather_code": [3,61,80,95,0,1,2],
    "sunrise": ["2026-04-21T06:30","2026-04-22T06:29","2026-04-23T06:28","2026-04-24T06:27","2026-04-25T06:26","2026-04-26T06:25","2026-04-27T06:24"],
    "sunset": ["2026-04-21T19:30","2026-04-22T19:31","2026-04-23T19:32","2026-04-24T19:33","2026-04-25T19:34","2026-04-26T19:35","2026-04-27T19:36"],
    "precipitation_sum": [0.0,1.2,5.0,10.0,0.0,0.0,0.0],
    "wind_speed_10m_max": [12.0,18.5,25.0,30.0,8.0,14.0,22.0]
  }
}`

// captureFetcher intercepts the outbound request URL and returns a canned body.
func newCapturingFetcher(t *testing.T) (*Fetcher, *url.URL) {
	t.Helper()
	captured := new(url.URL)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*captured = *r.URL
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(openMeteoSampleBody))
	}))
	t.Cleanup(srv.Close)

	f := New(nil)
	// Redirect the fetcher at our test server by swapping out the HTTP client's
	// transport to rewrite the host to the test server.
	f.client.Transport = &hostRewriter{target: srv.URL}
	return f, captured
}

type hostRewriter struct {
	target string
}

func (h *hostRewriter) RoundTrip(req *http.Request) (*http.Response, error) {
	u, err := url.Parse(h.target)
	if err != nil {
		return nil, err
	}
	req.URL.Scheme = u.Scheme
	req.URL.Host = u.Host
	return http.DefaultTransport.RoundTrip(req)
}

func TestFetchRequestsSevenDayForecastWithWind(t *testing.T) {
	f, captured := newCapturingFetcher(t)
	w := types.Weather{Latitude: 43.65, Longitude: -79.38, Units: "metric"}

	f.RefreshNow(context.Background(), w)

	q := captured.Query()
	if got := q.Get("forecast_days"); got != "7" {
		t.Fatalf("forecast_days = %q, want %q", got, "7")
	}
	daily := q.Get("daily")
	if !strings.Contains(daily, "wind_speed_10m_max") {
		t.Fatalf("daily query %q does not request wind_speed_10m_max", daily)
	}
}

func TestSnapshotIncludesWindForEachDay(t *testing.T) {
	f, _ := newCapturingFetcher(t)
	w := types.Weather{Latitude: 43.65, Longitude: -79.38, Units: "metric"}

	f.RefreshNow(context.Background(), w)

	snap := f.Snapshot()
	if snap == nil {
		t.Fatalf("snapshot is nil")
	}
	if len(snap.Daily) != 7 {
		t.Fatalf("len(daily) = %d, want 7", len(snap.Daily))
	}
	// Day 0: 12.0, Day 3: 30.0 -- from the canned response.
	if snap.Daily[0].WindSpeedMax != 12.0 {
		t.Errorf("daily[0].WindSpeedMax = %v, want 12.0", snap.Daily[0].WindSpeedMax)
	}
	if snap.Daily[3].WindSpeedMax != 30.0 {
		t.Errorf("daily[3].WindSpeedMax = %v, want 30.0", snap.Daily[3].WindSpeedMax)
	}
}

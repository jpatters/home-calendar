package weather

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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

const ecowittSampleBody = `{
  "common_list": [
    {"id": "0x02", "val": "6.3", "unit": "C"},
    {"id": "0x07", "val": "88%"},
    {"id": "0x0A", "val": "207"},
    {"id": "0x0B", "val": "3.2 m/s"},
    {"id": "0x0C", "val": "4.1 m/s"},
    {"id": "0x15", "val": "120.0 w/m2"}
  ],
  "wh25": [
    {"intemp": "21.0", "unit": "C", "inhumi": "44%", "abs": "1014.0 hPa", "rel": "1014.0 hPa"}
  ],
  "rain": [
    {"id": "0x0D", "val": "0.0 mm"},
    {"id": "0x0E", "val": "0.0 mm/Hr"},
    {"id": "0x10", "val": "2.5 mm"},
    {"id": "0x11", "val": "7.0 mm"},
    {"id": "0x12", "val": "30.0 mm"},
    {"id": "0x13", "val": "150.0 mm"}
  ]
}`

// newHybridFetcher returns a Fetcher pointed at a single test server which
// serves both Open-Meteo (/v1/forecast) and Ecowitt (/get_livedata_info)
// payloads from canned bodies. It returns a counter for Ecowitt calls and the
// Ecowitt URL to plug into config.
func newHybridFetcher(t *testing.T) (*Fetcher, *int32, string) {
	t.Helper()
	var ecowittCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/v1/forecast"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(openMeteoSampleBody))
		case strings.HasPrefix(r.URL.Path, "/get_livedata_info"):
			atomic.AddInt32(&ecowittCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(ecowittSampleBody))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	f := New(nil)
	// Redirect only the api.open-meteo.com host to the test server.
	f.client.Transport = &hostRewriter{target: srv.URL}
	return f, &ecowittCalls, srv.URL + "/get_livedata_info"
}

func TestFetcherOverlaysEcowittReadingsOnSnapshot(t *testing.T) {
	f, _, ecowittURL := newHybridFetcher(t)
	w := types.Weather{Latitude: 43.65, Longitude: -79.38, Units: "metric", EcowittURL: ecowittURL}

	prev := ecowittInterval
	ecowittInterval = 20 * time.Millisecond
	t.Cleanup(func() { ecowittInterval = prev })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.Start(ctx, w, time.Hour)
	defer f.Stop()

	if !waitFor(func() bool {
		s := f.Snapshot()
		return s != nil && s.Station != nil
	}, 2*time.Second) {
		t.Fatalf("timed out waiting for ecowitt overlay")
	}
	snap := f.Snapshot()
	if snap.Current.TemperatureC != 6.3 {
		t.Errorf("Current.TemperatureC = %v, want 6.3 (ecowitt overrides open-meteo's 14.2)", snap.Current.TemperatureC)
	}
	if snap.Current.Humidity != 88 {
		t.Errorf("Current.Humidity = %d, want 88 (ecowitt)", snap.Current.Humidity)
	}
	// 3.2 m/s -> 11.52 km/h (metric display unit, matching Open-Meteo)
	if got := snap.Current.WindSpeed; got < 11.5 || got > 11.6 {
		t.Errorf("Current.WindSpeed = %v, want ~11.52 km/h (ecowitt 3.2 m/s)", got)
	}
	if snap.Current.WeatherCode != 3 {
		t.Errorf("Current.WeatherCode = %d, want 3 (preserved from open-meteo)", snap.Current.WeatherCode)
	}
	if snap.Current.ApparentC != 13.0 {
		t.Errorf("Current.ApparentC = %v, want 13.0 (preserved from open-meteo)", snap.Current.ApparentC)
	}
	if len(snap.Daily) != 7 {
		t.Errorf("len(Daily) = %d, want 7 (preserved from open-meteo)", len(snap.Daily))
	}
}

func TestFetcherPopulatesStationDetailsOnSnapshot(t *testing.T) {
	f, _, ecowittURL := newHybridFetcher(t)
	w := types.Weather{Latitude: 43.65, Longitude: -79.38, Units: "metric", EcowittURL: ecowittURL}

	prev := ecowittInterval
	ecowittInterval = 20 * time.Millisecond
	t.Cleanup(func() { ecowittInterval = prev })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.Start(ctx, w, time.Hour)
	defer f.Stop()

	if !waitFor(func() bool {
		s := f.Snapshot()
		return s != nil && s.Station != nil
	}, 2*time.Second) {
		t.Fatalf("timed out waiting for ecowitt station")
	}
	st := f.Snapshot().Station
	if st == nil {
		t.Fatalf("Station is nil")
	}
	if st.IndoorTempC != 21.0 {
		t.Errorf("IndoorTempC = %v, want 21.0", st.IndoorTempC)
	}
	if st.IndoorHumidity != 44 {
		t.Errorf("IndoorHumidity = %d, want 44", st.IndoorHumidity)
	}
	if st.PressureHPa != 1014.0 {
		t.Errorf("PressureHPa = %v, want 1014.0", st.PressureHPa)
	}
	// 4.1 m/s -> 14.76 km/h
	if got := st.WindGust; got < 14.7 || got > 14.8 {
		t.Errorf("WindGust = %v, want ~14.76 km/h (ecowitt 4.1 m/s)", got)
	}
	if st.WindDirection != 207 {
		t.Errorf("WindDirection = %d, want 207", st.WindDirection)
	}
	if st.SolarWM2 != 120.0 {
		t.Errorf("SolarWM2 = %v, want 120.0", st.SolarWM2)
	}
	if st.RainDaily != 2.5 {
		t.Errorf("RainDaily = %v, want 2.5", st.RainDaily)
	}
	if st.RainYearly != 150.0 {
		t.Errorf("RainYearly = %v, want 150.0", st.RainYearly)
	}
}

func TestFetcherStationIsNilWhenEcowittURLEmpty(t *testing.T) {
	f, _, _ := newHybridFetcher(t)
	w := types.Weather{Latitude: 43.65, Longitude: -79.38, Units: "metric"} // no EcowittURL

	f.RefreshNow(context.Background(), w)

	snap := f.Snapshot()
	if snap == nil {
		t.Fatalf("snapshot is nil")
	}
	if snap.Station != nil {
		t.Errorf("Station = %+v, want nil when no EcowittURL", snap.Station)
	}
}

func TestFetcherSkipsEcowittWhenURLEmpty(t *testing.T) {
	f, calls, _ := newHybridFetcher(t)
	w := types.Weather{Latitude: 43.65, Longitude: -79.38, Units: "metric"}

	prev := ecowittInterval
	ecowittInterval = 20 * time.Millisecond
	t.Cleanup(func() { ecowittInterval = prev })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.Start(ctx, w, time.Hour)
	defer f.Stop()

	time.Sleep(100 * time.Millisecond)

	if got := atomic.LoadInt32(calls); got != 0 {
		t.Errorf("ecowitt was called %d times, want 0 when URL is empty", got)
	}
}

func TestFetcherEcowittPollsRepeatedly(t *testing.T) {
	f, calls, ecowittURL := newHybridFetcher(t)
	w := types.Weather{Latitude: 43.65, Longitude: -79.38, Units: "metric", EcowittURL: ecowittURL}

	prev := ecowittInterval
	ecowittInterval = 20 * time.Millisecond
	t.Cleanup(func() { ecowittInterval = prev })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.Start(ctx, w, time.Hour)
	defer f.Stop()

	if !waitFor(func() bool { return atomic.LoadInt32(calls) >= 3 }, 2*time.Second) {
		t.Fatalf("expected >=3 ecowitt calls within budget, got %d", atomic.LoadInt32(calls))
	}
}

func TestFetcherEmitsBroadcastWhenEcowittUpdates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/v1/forecast"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(openMeteoSampleBody))
		case strings.HasPrefix(r.URL.Path, "/get_livedata_info"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(ecowittSampleBody))
		}
	}))
	defer srv.Close()

	updates := make(chan *types.WeatherSnapshot, 16)
	f := New(func(s *types.WeatherSnapshot) {
		updates <- s
	})
	f.client.Transport = &hostRewriter{target: srv.URL}

	prev := ecowittInterval
	ecowittInterval = 20 * time.Millisecond
	t.Cleanup(func() { ecowittInterval = prev })

	w := types.Weather{Latitude: 43.65, Longitude: -79.38, Units: "metric", EcowittURL: srv.URL + "/get_livedata_info"}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.Start(ctx, w, time.Hour)
	defer f.Stop()

	deadline := time.After(2 * time.Second)
	for {
		select {
		case s := <-updates:
			if s != nil && s.Station != nil {
				return
			}
		case <-deadline:
			t.Fatalf("did not receive broadcast containing ecowitt station")
		}
	}
}

func TestFetcherRespectsConfiguredUnitsForOverlay(t *testing.T) {
	f, _, ecowittURL := newHybridFetcher(t)
	w := types.Weather{Latitude: 43.65, Longitude: -79.38, Units: "imperial", EcowittURL: ecowittURL}

	prev := ecowittInterval
	ecowittInterval = 20 * time.Millisecond
	t.Cleanup(func() { ecowittInterval = prev })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.Start(ctx, w, time.Hour)
	defer f.Stop()

	if !waitFor(func() bool {
		s := f.Snapshot()
		return s != nil && s.Station != nil
	}, 2*time.Second) {
		t.Fatalf("timed out waiting for ecowitt overlay")
	}
	snap := f.Snapshot()
	// 6.3 °C -> 43.34 °F
	if got := snap.Current.TemperatureC; got < 43 || got > 44 {
		t.Errorf("Current.TemperatureC = %v, want ~43.34 °F", got)
	}
	// 3.2 m/s -> ~7.16 mph
	if got := snap.Current.WindSpeed; got < 7 || got > 7.3 {
		t.Errorf("Current.WindSpeed = %v, want ~7.16 mph", got)
	}
}

func TestFetcherKeepsOpenMeteoPrecipitationContract(t *testing.T) {
	// Current.Precipitation is Open-Meteo's "amount over the last hour" (mm).
	// Ecowitt's rain-rate field is mm/h. Different semantics. The overlay
	// must NOT replace one with the other -- that would silently change the
	// meaning of a broadcast field. The live rain rate lives on Station.RainRate.
	const omBody = `{
  "timezone": "UTC",
  "current": {"time": "2026-04-21T12:00", "temperature_2m": 14.2, "apparent_temperature": 13.0, "relative_humidity_2m": 60, "wind_speed_10m": 11.0, "weather_code": 3, "is_day": 1, "precipitation": 2.5},
  "daily": {"time": ["2026-04-21"], "temperature_2m_max": [18.0], "temperature_2m_min": [8.0], "weather_code": [3], "sunrise": ["2026-04-21T06:30"], "sunset": ["2026-04-21T19:30"], "precipitation_sum": [0.0], "wind_speed_10m_max": [12.0]}
}`
	const ecBody = `{
  "common_list": [
    {"id": "0x02", "val": "6.3", "unit": "C"},
    {"id": "0x07", "val": "88%"},
    {"id": "0x0B", "val": "3.2 m/s"}
  ],
  "rain": [
    {"id": "0x0E", "val": "9.9 mm/Hr"}
  ]
}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/v1/forecast"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(omBody))
		case strings.HasPrefix(r.URL.Path, "/get_livedata_info"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(ecBody))
		}
	}))
	t.Cleanup(srv.Close)

	f := New(nil)
	f.client.Transport = &hostRewriter{target: srv.URL}

	prev := ecowittInterval
	ecowittInterval = 20 * time.Millisecond
	t.Cleanup(func() { ecowittInterval = prev })

	w := types.Weather{Latitude: 43.65, Longitude: -79.38, Units: "metric", EcowittURL: srv.URL + "/get_livedata_info"}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.Start(ctx, w, time.Hour)
	defer f.Stop()

	if !waitFor(func() bool {
		s := f.Snapshot()
		return s != nil && s.Station != nil
	}, 2*time.Second) {
		t.Fatalf("timed out waiting for ecowitt station")
	}
	snap := f.Snapshot()
	if snap.Current.Precipitation != 2.5 {
		t.Errorf("Current.Precipitation = %v, want 2.5 (preserved from open-meteo; not the 9.9 mm/Hr ecowitt rate)", snap.Current.Precipitation)
	}
	if snap.Station.RainRate != 9.9 {
		t.Errorf("Station.RainRate = %v, want 9.9 (ecowitt rain rate)", snap.Station.RainRate)
	}
}

func TestFetcherEcowittFailureLeavesOpenMeteoSnapshotIntact(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/v1/forecast"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(openMeteoSampleBody))
		case strings.HasPrefix(r.URL.Path, "/get_livedata_info"):
			http.Error(w, "boom", http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	f := New(nil)
	f.client.Transport = &hostRewriter{target: srv.URL}

	prev := ecowittInterval
	ecowittInterval = 20 * time.Millisecond
	t.Cleanup(func() { ecowittInterval = prev })

	w := types.Weather{Latitude: 43.65, Longitude: -79.38, Units: "metric", EcowittURL: srv.URL + "/get_livedata_info"}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.Start(ctx, w, time.Hour)
	defer f.Stop()

	if !waitFor(func() bool { return f.Snapshot() != nil }, 2*time.Second) {
		t.Fatalf("snapshot never populated")
	}
	snap := f.Snapshot()
	if snap.Current.TemperatureC != 14.2 {
		t.Errorf("Current.TemperatureC = %v, want 14.2 (open-meteo)", snap.Current.TemperatureC)
	}
	if snap.Station != nil {
		t.Errorf("Station = %+v, want nil when ecowitt fails", snap.Station)
	}
}

func waitFor(cond func() bool, budget time.Duration) bool {
	deadline := time.Now().Add(budget)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

package weather

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

type Fetcher struct {
	client *http.Client

	mu       sync.RWMutex
	snapshot *types.WeatherSnapshot
	lastErr  error

	cancel   context.CancelFunc
	doneWG   sync.WaitGroup
	onUpdate func(*types.WeatherSnapshot)
}

func New(onUpdate func(*types.WeatherSnapshot)) *Fetcher {
	return &Fetcher{
		client:   &http.Client{Timeout: 20 * time.Second},
		onUpdate: onUpdate,
	}
}

func (f *Fetcher) HTTPClient() *http.Client {
	return f.client
}

func (f *Fetcher) Snapshot() *types.WeatherSnapshot {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.snapshot == nil {
		return nil
	}
	s := *f.snapshot
	s.Daily = append([]types.WeatherDaily(nil), f.snapshot.Daily...)
	return &s
}

func (f *Fetcher) Start(parent context.Context, w types.Weather, interval time.Duration) {
	f.Stop()
	ctx, cancel := context.WithCancel(parent)
	f.cancel = cancel
	f.doneWG.Add(1)
	go f.loop(ctx, w, interval)
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

func (f *Fetcher) RefreshNow(ctx context.Context, w types.Weather) {
	f.fetch(ctx, w)
}

func (f *Fetcher) loop(ctx context.Context, w types.Weather, interval time.Duration) {
	defer f.doneWG.Done()
	f.fetch(ctx, w)
	if interval <= 0 {
		interval = 15 * time.Minute
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			f.fetch(ctx, w)
		}
	}
}

type openMeteoResponse struct {
	Timezone string `json:"timezone"`
	Current  struct {
		Time                string  `json:"time"`
		Temperature2m       float64 `json:"temperature_2m"`
		ApparentTemperature float64 `json:"apparent_temperature"`
		RelativeHumidity2m  int     `json:"relative_humidity_2m"`
		WindSpeed10m        float64 `json:"wind_speed_10m"`
		WeatherCode         int     `json:"weather_code"`
		IsDay               int     `json:"is_day"`
		Precipitation       float64 `json:"precipitation"`
	} `json:"current"`
	Daily struct {
		Time             []string  `json:"time"`
		Temperature2mMax []float64 `json:"temperature_2m_max"`
		Temperature2mMin []float64 `json:"temperature_2m_min"`
		WeatherCode      []int     `json:"weather_code"`
		Sunrise          []string  `json:"sunrise"`
		Sunset           []string  `json:"sunset"`
		PrecipitationSum []float64 `json:"precipitation_sum"`
	} `json:"daily"`
}

func (f *Fetcher) fetch(ctx context.Context, w types.Weather) {
	u, _ := url.Parse("https://api.open-meteo.com/v1/forecast")
	q := u.Query()
	q.Set("latitude", strconv.FormatFloat(w.Latitude, 'f', 4, 64))
	q.Set("longitude", strconv.FormatFloat(w.Longitude, 'f', 4, 64))
	q.Set("current", "temperature_2m,apparent_temperature,relative_humidity_2m,wind_speed_10m,weather_code,is_day,precipitation")
	q.Set("daily", "temperature_2m_max,temperature_2m_min,weather_code,sunrise,sunset,precipitation_sum")
	q.Set("forecast_days", "4")
	q.Set("timezone", defaultString(w.Timezone, "auto"))
	if w.Units == "imperial" {
		q.Set("temperature_unit", "fahrenheit")
		q.Set("wind_speed_unit", "mph")
		q.Set("precipitation_unit", "inch")
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		f.setErr(err)
		return
	}
	resp, err := f.client.Do(req)
	if err != nil {
		f.setErr(err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		f.setErr(fmt.Errorf("open-meteo http %d", resp.StatusCode))
		return
	}
	var body openMeteoResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		f.setErr(err)
		return
	}
	snap := toSnapshot(body, w)

	f.mu.Lock()
	f.snapshot = snap
	f.lastErr = nil
	f.mu.Unlock()
	if f.onUpdate != nil {
		f.onUpdate(snap)
	}
}

func (f *Fetcher) setErr(err error) {
	log.Printf("weather: %v", err)
	f.mu.Lock()
	f.lastErr = err
	f.mu.Unlock()
}

func toSnapshot(r openMeteoResponse, w types.Weather) *types.WeatherSnapshot {
	snap := &types.WeatherSnapshot{
		UpdatedAt: time.Now(),
		Units:     defaultString(w.Units, "metric"),
		Timezone:  r.Timezone,
	}
	if t, err := time.Parse("2006-01-02T15:04", r.Current.Time); err == nil {
		snap.Current.Time = t
	}
	snap.Current.TemperatureC = r.Current.Temperature2m
	snap.Current.ApparentC = r.Current.ApparentTemperature
	snap.Current.Humidity = r.Current.RelativeHumidity2m
	snap.Current.WindSpeed = r.Current.WindSpeed10m
	snap.Current.WeatherCode = r.Current.WeatherCode
	snap.Current.IsDay = r.Current.IsDay == 1
	snap.Current.Precipitation = r.Current.Precipitation

	n := len(r.Daily.Time)
	for i := 0; i < n; i++ {
		d := types.WeatherDaily{Date: r.Daily.Time[i]}
		if i < len(r.Daily.Temperature2mMax) {
			d.MaxC = r.Daily.Temperature2mMax[i]
		}
		if i < len(r.Daily.Temperature2mMin) {
			d.MinC = r.Daily.Temperature2mMin[i]
		}
		if i < len(r.Daily.WeatherCode) {
			d.WeatherCode = r.Daily.WeatherCode[i]
		}
		if i < len(r.Daily.Sunrise) {
			d.Sunrise = r.Daily.Sunrise[i]
		}
		if i < len(r.Daily.Sunset) {
			d.Sunset = r.Daily.Sunset[i]
		}
		if i < len(r.Daily.PrecipitationSum) {
			d.PrecipMM = r.Daily.PrecipitationSum[i]
		}
		snap.Daily = append(snap.Daily, d)
	}
	return snap
}

func defaultString(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

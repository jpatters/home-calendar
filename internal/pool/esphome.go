package pool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jpatters/home-calendar/internal/types"
)

// climatePath is the ESPHome web_server REST path for the pool heater climate
// entity. ESPHome addresses entities by their `name:` ("Pool Heater"); the
// space is percent-encoded by url.URL when the request is built.
const climatePath = "/climate/Pool Heater"

// Setpoint limits used when the thermostat does not report its own.
const (
	defaultMinTargetC = 10.0
	defaultMaxTargetC = 40.0
)

// ErrInvalidTarget reports a requested setpoint the pool heater cannot be set to.
var ErrInvalidTarget = errors.New("pool: invalid target temperature")

type Fetcher struct {
	client *http.Client

	// ioMu serialises polls and writes so a poll that started before a write
	// cannot publish its older reading after the write's.
	ioMu     sync.Mutex
	mu       sync.RWMutex
	snapshot *types.PoolSnapshot
	lastErr  error

	cancel   context.CancelFunc
	doneWG   sync.WaitGroup
	onUpdate func(*types.PoolSnapshot)
}

func New(onUpdate func(*types.PoolSnapshot)) *Fetcher {
	return &Fetcher{
		client:   &http.Client{Timeout: 5 * time.Second},
		onUpdate: onUpdate,
	}
}

func (f *Fetcher) Snapshot() *types.PoolSnapshot {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.snapshot == nil {
		return nil
	}
	s := *f.snapshot
	return &s
}

func (f *Fetcher) Start(parent context.Context, p types.Pool, interval time.Duration) {
	f.Stop()
	ctx, cancel := context.WithCancel(parent)
	f.cancel = cancel
	f.doneWG.Add(1)
	go f.loop(ctx, p, interval)
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

func (f *Fetcher) RefreshNow(ctx context.Context, p types.Pool) {
	f.fetch(ctx, p)
}

// SetTarget changes the pool heater's setpoint and publishes the reading taken
// afterwards.
func (f *Fetcher) SetTarget(ctx context.Context, p types.Pool, targetC float64) (*types.PoolSnapshot, error) {
	f.ioMu.Lock()
	defer f.ioMu.Unlock()
	snap, err := SetTarget(ctx, f.client, p.DeviceURL, targetC)
	if err != nil {
		return nil, err
	}
	f.publish(snap)
	return snap, nil
}

func (f *Fetcher) publish(snap *types.PoolSnapshot) {
	f.mu.Lock()
	f.snapshot = snap
	f.lastErr = nil
	f.mu.Unlock()
	if f.onUpdate != nil {
		f.onUpdate(snap)
	}
}

func (f *Fetcher) loop(ctx context.Context, p types.Pool, interval time.Duration) {
	defer f.doneWG.Done()
	f.fetch(ctx, p)
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			f.fetch(ctx, p)
		}
	}
}

func (f *Fetcher) fetch(ctx context.Context, p types.Pool) {
	if strings.TrimSpace(p.DeviceURL) == "" {
		f.mu.Lock()
		hadSnapshot := f.snapshot != nil
		f.snapshot = nil
		f.mu.Unlock()
		if hadSnapshot && f.onUpdate != nil {
			f.onUpdate(nil)
		}
		return
	}
	f.ioMu.Lock()
	defer f.ioMu.Unlock()
	snap, err := Search(ctx, f.client, p.DeviceURL)
	if err != nil {
		// The ESP reboots/OTAs and drops wifi periodically; keep the last
		// good snapshot and try again on the next tick rather than clearing.
		log.Printf("pool: %v", err)
		f.mu.Lock()
		f.lastErr = err
		f.mu.Unlock()
		return
	}
	f.publish(snap)
}

type climateResponse struct {
	Action             string `json:"action"`
	CurrentTemperature string `json:"current_temperature"`
	TargetTemperature  string `json:"target_temperature"`
	MinTemp            string `json:"min_temp"`
	MaxTemp            string `json:"max_temp"`
}

// Search reads the pool heater's climate entity from an ESPHome device's
// web_server REST API at baseURL and returns a snapshot with the current water
// temperature, target, and whether the heat pump is actively heating.
//
// ESPHome serializes the climate temperatures as JSON strings (e.g. "25.2"),
// so they are parsed as floats here. The `action` field ("HEATING" vs "IDLE")
// is the live heat-call signal; `mode` only reflects setpoint intent.
// ESPHome only includes the setpoint limits (min_temp/max_temp) in the
// detail=all form of the response, so that is what is requested; when they
// are absent the pool defaults of 10–40 °C apply.
func Search(ctx context.Context, client *http.Client, baseURL string) (*types.PoolSnapshot, error) {
	if client == nil {
		client = http.DefaultClient
	}
	u, err := climateURL(baseURL, "")
	if err != nil {
		return nil, err
	}
	u.RawQuery = url.Values{"detail": {"all"}}.Encode()

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
		return nil, fmt.Errorf("pool: esphome http %d", resp.StatusCode)
	}
	var body climateResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("pool: decode response: %w", err)
	}

	current, err := strconv.ParseFloat(strings.TrimSpace(body.CurrentTemperature), 64)
	if err != nil {
		return nil, fmt.Errorf("pool: parse current_temperature %q: %w", body.CurrentTemperature, err)
	}
	// ESPHome reports "nan" for current_temperature while the probe is
	// unavailable (e.g. during a reboot). ParseFloat accepts that as a valid
	// NaN, so reject non-finite values explicitly and keep the last good
	// snapshot rather than broadcasting a bogus reading.
	if math.IsNaN(current) || math.IsInf(current, 0) {
		return nil, fmt.Errorf("pool: current_temperature not finite: %q", body.CurrentTemperature)
	}
	// Target is best-effort: a missing/blank/non-finite setpoint shouldn't drop
	// the whole reading, since the temperature and heating state are the
	// primary signal. A zero target is treated as "no setpoint" by the UI.
	target, _ := strconv.ParseFloat(strings.TrimSpace(body.TargetTemperature), 64)
	if math.IsNaN(target) || math.IsInf(target, 0) {
		target = 0
	}

	return &types.PoolSnapshot{
		UpdatedAt:    time.Now(),
		TemperatureC: current,
		TargetC:      target,
		MinTargetC:   parseLimit(body.MinTemp, defaultMinTargetC),
		MaxTargetC:   parseLimit(body.MaxTemp, defaultMaxTargetC),
		Heating:      body.Action == "HEATING",
	}, nil
}

// SetTarget sets the pool heater's setpoint to targetC, which must lie within
// the limits the thermostat reports (inclusive), and returns the reading taken
// after the change. Out-of-range targets are rejected with ErrInvalidTarget
// without writing to the device.
func SetTarget(ctx context.Context, client *http.Client, baseURL string, targetC float64) (*types.PoolSnapshot, error) {
	if client == nil {
		client = http.DefaultClient
	}
	before, err := Search(ctx, client, baseURL)
	if err != nil {
		return nil, err
	}
	if math.IsNaN(targetC) || targetC < before.MinTargetC || targetC > before.MaxTargetC {
		return nil, fmt.Errorf("%w: %v°C is outside %v–%v°C", ErrInvalidTarget, targetC, before.MinTargetC, before.MaxTargetC)
	}

	u, err := climateURL(baseURL, "/set")
	if err != nil {
		return nil, err
	}
	u.RawQuery = url.Values{"target_temperature": {strconv.FormatFloat(targetC, 'f', -1, 64)}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pool: esphome set http %d", resp.StatusCode)
	}
	return Search(ctx, client, baseURL)
}

func climateURL(baseURL, suffix string) (*url.URL, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("pool: parse device URL: %w", err)
	}
	u.Path = strings.TrimRight(u.Path, "/") + climatePath + suffix
	return u, nil
}

func parseLimit(s string, fallback float64) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return fallback
	}
	return v
}

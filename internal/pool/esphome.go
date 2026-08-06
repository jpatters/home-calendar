package pool

import (
	"context"
	"encoding/json"
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
// entity. The object id is derived from the entity's `name:` ("Pool Heater");
// the space is percent-encoded by url.URL when the request is built.
const climatePath = "/climate/Pool Heater"

type Fetcher struct {
	client *http.Client

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
	f.mu.Lock()
	f.snapshot = snap
	f.lastErr = nil
	f.mu.Unlock()
	if f.onUpdate != nil {
		f.onUpdate(snap)
	}
}

type climateResponse struct {
	Action             string `json:"action"`
	CurrentTemperature string `json:"current_temperature"`
	TargetTemperature  string `json:"target_temperature"`
}

// Search reads the pool heater's climate entity from an ESPHome device's
// web_server REST API at baseURL and returns a snapshot with the current water
// temperature, target, and whether the heat pump is actively heating.
//
// ESPHome serializes the climate temperatures as JSON strings (e.g. "25.2"),
// so they are parsed as floats here. The `action` field ("HEATING" vs "IDLE")
// is the live heat-call signal; `mode` only reflects setpoint intent.
func Search(ctx context.Context, client *http.Client, baseURL string) (*types.PoolSnapshot, error) {
	if client == nil {
		client = http.DefaultClient
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("pool: parse device URL: %w", err)
	}
	u.Path = strings.TrimRight(u.Path, "/") + climatePath

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
		Heating:      body.Action == "HEATING",
	}, nil
}

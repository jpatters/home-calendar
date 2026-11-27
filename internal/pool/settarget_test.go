package pool_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"testing"

	"github.com/jpatters/home-calendar/internal/pool"
	"github.com/jpatters/home-calendar/internal/types"
)

// fakeThermostat emulates the ESPHome web_server climate entity "Pool Heater".
// Like the real device it only includes min_temp/max_temp when the GET asks
// for ?detail=all, serializes temperatures as strings, and accepts setpoint
// changes at POST /climate/<name>/set?target_temperature=<v>.
type fakeThermostat struct {
	mu        sync.Mutex
	target    float64
	limits    *[2]float64
	setStatus int
	sets      []string // raw query of each set request received
}

func (d *fakeThermostat) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	defer d.mu.Unlock()
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/climate/Pool Heater":
		limits := ""
		if d.limits != nil && r.URL.Query().Get("detail") == "all" {
			limits = fmt.Sprintf(`,"min_temp":"%.1f","max_temp":"%.1f","step":0.5`, d.limits[0], d.limits[1])
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"name_id":"climate/Pool Heater","id":"climate/Pool Heater","mode":"HEAT","action":"IDLE","state":"HEAT","current_temperature":"25.2","target_temperature":"%.1f"%s}`, d.target, limits)
	case r.Method == http.MethodPost && r.URL.Path == "/climate/Pool Heater/set":
		d.sets = append(d.sets, r.URL.RawQuery)
		if d.setStatus != 0 && d.setStatus != http.StatusOK {
			http.Error(w, "nope", d.setStatus)
			return
		}
		v, err := strconv.ParseFloat(r.URL.Query().Get("target_temperature"), 64)
		if err != nil {
			http.Error(w, "bad target", http.StatusBadRequest)
			return
		}
		d.target = v
		w.WriteHeader(http.StatusOK)
	default:
		http.NotFound(w, r)
	}
}

func (d *fakeThermostat) setRequests() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.sets...)
}

func startThermostat(t *testing.T, d *fakeThermostat) types.Pool {
	t.Helper()
	srv := httptest.NewServer(d)
	t.Cleanup(srv.Close)
	return types.Pool{Enabled: true, DeviceURL: srv.URL}
}

func TestSetTargetSendsTargetToClimateSetEndpoint(t *testing.T) {
	dev := &fakeThermostat{target: 27, limits: &[2]float64{15, 35}}
	p := startThermostat(t, dev)

	if _, err := pool.New(nil).SetTarget(context.Background(), p, 28.5); err != nil {
		t.Fatalf("SetTarget: %v", err)
	}

	sets := dev.setRequests()
	if len(sets) != 1 {
		t.Fatalf("expected exactly one set request, got %d: %v", len(sets), sets)
	}
	q, err := url.ParseQuery(sets[0])
	if err != nil {
		t.Fatalf("parse set query %q: %v", sets[0], err)
	}
	if v, err := strconv.ParseFloat(q.Get("target_temperature"), 64); err != nil || v != 28.5 {
		t.Errorf("set query = %q, want target_temperature=28.5", sets[0])
	}
}

func TestSetTargetReturnsAndPublishesReadingTakenAfterWrite(t *testing.T) {
	dev := &fakeThermostat{target: 27, limits: &[2]float64{15, 35}}
	p := startThermostat(t, dev)

	var published []*types.PoolSnapshot
	f := pool.New(func(s *types.PoolSnapshot) { published = append(published, s) })

	snap, err := f.SetTarget(context.Background(), p, 30)
	if err != nil {
		t.Fatalf("SetTarget: %v", err)
	}
	if snap == nil || snap.TargetC != 30 {
		t.Fatalf("returned snapshot = %+v, want TargetC 30", snap)
	}
	if snap.TemperatureC != 25.2 {
		t.Errorf("TemperatureC = %v, want 25.2", snap.TemperatureC)
	}
	if len(published) != 1 || published[0] == nil || published[0].TargetC != 30 {
		t.Fatalf("published = %+v, want one snapshot with TargetC 30", published)
	}
	if cached := f.Snapshot(); cached == nil || cached.TargetC != 30 {
		t.Errorf("Snapshot() = %+v, want TargetC 30", cached)
	}
}

func TestSetTargetRejectsTargetOutsideDeviceLimitsWithoutWriting(t *testing.T) {
	dev := &fakeThermostat{target: 27, limits: &[2]float64{15, 35}}
	p := startThermostat(t, dev)

	var published int
	f := pool.New(func(*types.PoolSnapshot) { published++ })

	for _, target := range []float64{14.5, 35.5} {
		snap, err := f.SetTarget(context.Background(), p, target)
		if !errors.Is(err, pool.ErrInvalidTarget) {
			t.Errorf("target %v: err = %v, want ErrInvalidTarget", target, err)
		}
		if snap != nil {
			t.Errorf("target %v: expected nil snapshot, got %+v", target, snap)
		}
	}
	if sets := dev.setRequests(); len(sets) != 0 {
		t.Errorf("expected no set requests, got %v", sets)
	}
	if published != 0 {
		t.Errorf("expected nothing published, got %d", published)
	}
}

func TestSetTargetUsesDefaultLimitsWhenDeviceReportsNone(t *testing.T) {
	dev := &fakeThermostat{target: 27}
	p := startThermostat(t, dev)
	f := pool.New(nil)

	for _, target := range []float64{9.5, 40.5} {
		if _, err := f.SetTarget(context.Background(), p, target); !errors.Is(err, pool.ErrInvalidTarget) {
			t.Errorf("target %v: err = %v, want ErrInvalidTarget", target, err)
		}
	}
	if sets := dev.setRequests(); len(sets) != 0 {
		t.Errorf("expected no set requests for out-of-range targets, got %v", sets)
	}

	snap, err := f.SetTarget(context.Background(), p, 40)
	if err != nil {
		t.Fatalf("SetTarget(40): %v", err)
	}
	if snap.TargetC != 40 {
		t.Errorf("TargetC = %v, want 40", snap.TargetC)
	}
}

func TestSetTargetReturnsErrorWhenDeviceRejectsWrite(t *testing.T) {
	dev := &fakeThermostat{target: 27, limits: &[2]float64{15, 35}, setStatus: http.StatusInternalServerError}
	p := startThermostat(t, dev)

	var published int
	f := pool.New(func(*types.PoolSnapshot) { published++ })

	snap, err := f.SetTarget(context.Background(), p, 30)
	if err == nil {
		t.Fatalf("expected error, got snapshot %+v", snap)
	}
	if errors.Is(err, pool.ErrInvalidTarget) {
		t.Errorf("device failure must not be reported as ErrInvalidTarget: %v", err)
	}
	if published != 0 {
		t.Errorf("expected nothing published on failure, got %d", published)
	}
}

func TestSearchReportsDeviceTargetLimits(t *testing.T) {
	dev := &fakeThermostat{target: 27, limits: &[2]float64{15, 35}}
	p := startThermostat(t, dev)

	snap, err := pool.Search(context.Background(), nil, p.DeviceURL)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if snap.MinTargetC != 15 || snap.MaxTargetC != 35 {
		t.Errorf("limits = %v–%v, want 15–35", snap.MinTargetC, snap.MaxTargetC)
	}
}

func TestSearchFallsBackToDefaultLimitsWhenDeviceReportsNone(t *testing.T) {
	dev := &fakeThermostat{target: 27}
	p := startThermostat(t, dev)

	snap, err := pool.Search(context.Background(), nil, p.DeviceURL)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if snap.MinTargetC != 10 || snap.MaxTargetC != 40 {
		t.Errorf("limits = %v–%v, want default 10–40", snap.MinTargetC, snap.MaxTargetC)
	}
}

func TestSetTargetAcceptsTargetsExactlyAtLimits(t *testing.T) {
	dev := &fakeThermostat{target: 27, limits: &[2]float64{15, 35}}
	p := startThermostat(t, dev)
	f := pool.New(nil)

	for _, target := range []float64{15, 35} {
		snap, err := f.SetTarget(context.Background(), p, target)
		if err != nil {
			t.Fatalf("SetTarget(%v): %v", target, err)
		}
		if snap.TargetC != target {
			t.Errorf("TargetC = %v, want %v", snap.TargetC, target)
		}
	}
}

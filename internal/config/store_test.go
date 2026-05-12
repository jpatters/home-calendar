package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jpatters/home-calendar/internal/types"
)

func TestReplaceRoundTripsConfig(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	input := types.Config{
		Calendars: []types.Calendar{
			{ID: "cal-1", Name: "Family", Color: "#ff0000", URL: "https://example.com/a.ics"},
		},
		Weather: types.Weather{
			Latitude:  49.2827,
			Longitude: -123.1207,
			Units:     "imperial",
			Timezone:  "America/Vancouver",
			Location:  "Vancouver, BC, Canada",
		},
		Tide: types.Tide{
			Latitude:  48.4284,
			Longitude: -123.3656,
			Units:     "imperial",
			Timezone:  "America/Vancouver",
			Location:  "Victoria, BC, Canada",
		},
		SnowDay: types.SnowDay{URL: "https://example.com/snow"},
		Display: types.Display{
			DefaultView:            "day",
			CalendarRefreshSeconds: 120,
			WeatherRefreshSeconds:  600,
			TideRefreshSeconds:     1800,
			BaseballRefreshSeconds: 900,
			Theme:                  "ocean",
			Mode:                   "dark",
		},
	}

	saved, err := s.Replace(input)
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}
	if !reflect.DeepEqual(saved, input) {
		t.Errorf("Replace return value differed from input.\ninput=%+v\nsaved=%+v", input, saved)
	}

	reopened, err := Open(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got := reopened.Get()
	if !reflect.DeepEqual(got, input) {
		t.Errorf("persisted config differed from input.\ninput=%+v\ngot=%+v", input, got)
	}
}

func TestNormalizeFillsTideDefaultsWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	cfg := s.Get()
	if cfg.Tide.Units != "metric" {
		t.Errorf("Tide.Units = %q, want %q", cfg.Tide.Units, "metric")
	}
	if cfg.Tide.Timezone == "" {
		t.Errorf("Tide.Timezone should default to non-empty, got empty")
	}
	if cfg.Display.TideRefreshSeconds <= 0 {
		t.Errorf("Display.TideRefreshSeconds = %d, want positive default",
			cfg.Display.TideRefreshSeconds)
	}
}

func TestNormalizeFillsThemeAndModeDefaultsWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got := s.Get().Display
	if got.Theme != "default" {
		t.Errorf("Theme = %q, want %q", got.Theme, "default")
	}
	if got.Mode != "light" {
		t.Errorf("Mode = %q, want %q", got.Mode, "light")
	}
}

func TestNormalizeMigratesLegacyDarkTheme(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"display":{"theme":"dark"}}`), 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got := s.Get().Display
	if got.Theme != "default" {
		t.Errorf("Theme = %q, want %q", got.Theme, "default")
	}
	if got.Mode != "dark" {
		t.Errorf("Mode = %q, want %q", got.Mode, "dark")
	}
}

func TestNormalizeMigratesLegacyLightTheme(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"display":{"theme":"light"}}`), 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got := s.Get().Display
	if got.Theme != "default" {
		t.Errorf("Theme = %q, want %q", got.Theme, "default")
	}
	if got.Mode != "light" {
		t.Errorf("Mode = %q, want %q", got.Mode, "light")
	}
}

func TestNormalizePreservesValidPaletteAndAutoMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"display":{"theme":"ocean","mode":"auto"}}`), 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got := s.Get().Display
	if got.Theme != "ocean" {
		t.Errorf("Theme = %q, want %q", got.Theme, "ocean")
	}
	if got.Mode != "auto" {
		t.Errorf("Mode = %q, want %q", got.Mode, "auto")
	}
}

func TestNormalizeFallsBackOnUnknownPalette(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"display":{"theme":"neon","mode":"dark"}}`), 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got := s.Get().Display
	if got.Theme != "default" {
		t.Errorf("Theme = %q, want %q", got.Theme, "default")
	}
	if got.Mode != "dark" {
		t.Errorf("Mode = %q, want %q", got.Mode, "dark")
	}
}

func TestBackfillEnabledDefaultsTrueWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	raw := `{"weather":{"latitude":1,"longitude":2,"units":"metric","timezone":"UTC","location":"x"},"tide":{"latitude":3,"longitude":4},"snowDay":{"url":"z"},"display":{}}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got := s.Get()
	if !got.Weather.Enabled {
		t.Errorf("Weather.Enabled = false, want true")
	}
	if !got.Tide.Enabled {
		t.Errorf("Tide.Enabled = false, want true")
	}
	if !got.SnowDay.Enabled {
		t.Errorf("SnowDay.Enabled = false, want true")
	}
	if !got.Display.CalendarEnabled {
		t.Errorf("Display.CalendarEnabled = false, want true")
	}
	if !got.Display.ClockEnabled {
		t.Errorf("Display.ClockEnabled = false, want true")
	}
}

func TestBackfillPreservesExplicitFalseAndFillsAbsentFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	// Weather is explicitly disabled; tide present but without enabled;
	// snowDay absent entirely; display present but without the new fields.
	raw := `{"weather":{"enabled":false,"latitude":1,"longitude":2},"tide":{"latitude":3,"longitude":4},"display":{"theme":"ocean"}}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got := s.Get()
	if got.Weather.Enabled {
		t.Errorf("Weather.Enabled = true, want false (explicit)")
	}
	if !got.Tide.Enabled {
		t.Errorf("Tide.Enabled = false, want true (absent field)")
	}
	if !got.SnowDay.Enabled {
		t.Errorf("SnowDay.Enabled = false, want true (absent section)")
	}
	if !got.Display.CalendarEnabled {
		t.Errorf("Display.CalendarEnabled = false, want true (absent field)")
	}
	if !got.Display.ClockEnabled {
		t.Errorf("Display.ClockEnabled = false, want true (absent field)")
	}
}

func TestReplacePreservesExplicitlyDisabledFlags(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	in := s.Get()
	in.Weather.Enabled = false
	in.Tide.Enabled = false
	in.SnowDay.Enabled = false
	in.Display.CalendarEnabled = false
	in.Display.ClockEnabled = false
	if _, err := s.Replace(in); err != nil {
		t.Fatalf("Replace: %v", err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got := reopened.Get()
	if got.Weather.Enabled {
		t.Errorf("Weather.Enabled = true, want false")
	}
	if got.Tide.Enabled {
		t.Errorf("Tide.Enabled = true, want false")
	}
	if got.SnowDay.Enabled {
		t.Errorf("SnowDay.Enabled = true, want false")
	}
	if got.Display.CalendarEnabled {
		t.Errorf("Display.CalendarEnabled = true, want false")
	}
	if got.Display.ClockEnabled {
		t.Errorf("Display.ClockEnabled = true, want false")
	}
}

func TestEcowittURLRoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	in := s.Get()
	in.Weather.EcowittURL = "http://192.0.2.42/get_livedata_info"
	if _, err := s.Replace(in); err != nil {
		t.Fatalf("Replace: %v", err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if got := reopened.Get().Weather.EcowittURL; got != in.Weather.EcowittURL {
		t.Errorf("EcowittURL = %q, want %q", got, in.Weather.EcowittURL)
	}
}

func TestNormalizeFallsBackOnUnknownMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"display":{"theme":"forest","mode":"sunset"}}`), 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	cfg := s.Get()
	if _, err := s.Replace(cfg); err != nil {
		t.Fatalf("Replace: %v", err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got := reopened.Get().Display
	if got.Theme != "forest" {
		t.Errorf("Theme = %q, want %q", got.Theme, "forest")
	}
	if got.Mode != "light" {
		t.Errorf("Mode = %q, want %q", got.Mode, "light")
	}
}

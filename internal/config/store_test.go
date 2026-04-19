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
		SnowDay: types.SnowDay{URL: "https://example.com/snow"},
		Display: types.Display{
			DefaultView:            "day",
			CalendarRefreshSeconds: 120,
			WeatherRefreshSeconds:  600,
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

package config

import (
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
			Theme:                  "dark",
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

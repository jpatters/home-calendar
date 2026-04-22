package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/jpatters/home-calendar/internal/types"
)

type Store struct {
	path string
	mu   sync.RWMutex
	cfg  types.Config
}

func Open(path string) (*Store, error) {
	s := &Store{path: path}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		s.cfg = types.DefaultConfig()
		return s.save(s.cfg)
	}
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	var c types.Config
	if err := json.Unmarshal(data, &c); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	backfillEnabledFlags(data, &c)
	s.cfg = normalize(c)
	return nil
}

// backfillEnabledFlags preserves the default-on behaviour for widgets on
// configs written before the enabled flags existed. Go's zero-value for bool is
// false, so a JSON file missing these keys would otherwise parse as "every
// widget disabled" and give a returning user a blank dashboard.
func backfillEnabledFlags(raw []byte, c *types.Config) {
	var probe struct {
		Weather *struct {
			Enabled *bool `json:"enabled"`
		} `json:"weather"`
		Tide *struct {
			Enabled *bool `json:"enabled"`
		} `json:"tide"`
		SnowDay *struct {
			Enabled *bool `json:"enabled"`
		} `json:"snowDay"`
		Baseball *struct {
			Enabled *bool `json:"enabled"`
		} `json:"baseball"`
		Display *struct {
			CalendarEnabled *bool `json:"calendarEnabled"`
			ClockEnabled    *bool `json:"clockEnabled"`
		} `json:"display"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return
	}
	if probe.Weather == nil || probe.Weather.Enabled == nil {
		c.Weather.Enabled = true
	}
	if probe.Tide == nil || probe.Tide.Enabled == nil {
		c.Tide.Enabled = true
	}
	if probe.SnowDay == nil || probe.SnowDay.Enabled == nil {
		c.SnowDay.Enabled = true
	}
	if probe.Baseball == nil || probe.Baseball.Enabled == nil {
		c.Baseball.Enabled = true
	}
	if probe.Display == nil || probe.Display.CalendarEnabled == nil {
		c.Display.CalendarEnabled = true
	}
	if probe.Display == nil || probe.Display.ClockEnabled == nil {
		c.Display.ClockEnabled = true
	}
}

func (s *Store) Get() types.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneConfig(s.cfg)
}

func (s *Store) Replace(c types.Config) (types.Config, error) {
	c = normalize(c)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.save(c); err != nil {
		return types.Config{}, err
	}
	s.cfg = c
	return cloneConfig(c), nil
}

func (s *Store) save(c types.Config) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func normalize(c types.Config) types.Config {
	d := types.DefaultConfig()
	if c.Calendars == nil {
		c.Calendars = []types.Calendar{}
	}
	for i, cal := range c.Calendars {
		if cal.ID == "" {
			cal.ID = uuid.NewString()
		}
		if cal.Color == "" {
			cal.Color = "#4285f4"
		}
		if cal.Name == "" {
			cal.Name = "Calendar"
		}
		c.Calendars[i] = cal
	}
	if c.Weather.Units == "" {
		c.Weather.Units = d.Weather.Units
	}
	if c.Weather.Timezone == "" {
		c.Weather.Timezone = d.Weather.Timezone
	}
	if c.Tide.Units == "" {
		c.Tide.Units = d.Tide.Units
	}
	if c.Tide.Timezone == "" {
		c.Tide.Timezone = d.Tide.Timezone
	}
	if c.Display.DefaultView == "" {
		c.Display.DefaultView = d.Display.DefaultView
	}
	if c.Display.CalendarRefreshSeconds <= 0 {
		c.Display.CalendarRefreshSeconds = d.Display.CalendarRefreshSeconds
	}
	if c.Display.WeatherRefreshSeconds <= 0 {
		c.Display.WeatherRefreshSeconds = d.Display.WeatherRefreshSeconds
	}
	if c.Display.TideRefreshSeconds <= 0 {
		c.Display.TideRefreshSeconds = d.Display.TideRefreshSeconds
	}
	if c.Display.BaseballRefreshSeconds <= 0 {
		c.Display.BaseballRefreshSeconds = d.Display.BaseballRefreshSeconds
	}
	c.Baseball.TeamName = strings.TrimSpace(c.Baseball.TeamName)
	c.Baseball.TeamAbbr = strings.TrimSpace(c.Baseball.TeamAbbr)
	c.Display = normalizeDisplayTheme(c.Display, d.Display)
	return c
}

var validPalettes = map[string]struct{}{
	"default": {},
	"ocean":   {},
	"sunset":  {},
	"forest":  {},
}

var validModes = map[string]struct{}{
	"light": {},
	"dark":  {},
	"auto":  {},
}

func normalizeDisplayTheme(cur, def types.Display) types.Display {
	if cur.Theme == "light" || cur.Theme == "dark" {
		if cur.Mode == "" {
			cur.Mode = cur.Theme
		}
		cur.Theme = "default"
	}
	if _, ok := validPalettes[cur.Theme]; !ok {
		if cur.Theme != "" {
			log.Printf("config: unknown theme %q, falling back to %q", cur.Theme, def.Theme)
		}
		cur.Theme = def.Theme
	}
	if _, ok := validModes[cur.Mode]; !ok {
		if cur.Mode != "" {
			log.Printf("config: unknown mode %q, falling back to %q", cur.Mode, def.Mode)
		}
		cur.Mode = def.Mode
	}
	return cur
}

func cloneConfig(c types.Config) types.Config {
	cals := make([]types.Calendar, len(c.Calendars))
	copy(cals, c.Calendars)
	c.Calendars = cals
	return c
}

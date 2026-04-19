package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	s.cfg = normalize(c)
	return nil
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
	if c.Display.DefaultView == "" {
		c.Display.DefaultView = d.Display.DefaultView
	}
	if c.Display.CalendarRefreshSeconds <= 0 {
		c.Display.CalendarRefreshSeconds = d.Display.CalendarRefreshSeconds
	}
	if c.Display.WeatherRefreshSeconds <= 0 {
		c.Display.WeatherRefreshSeconds = d.Display.WeatherRefreshSeconds
	}
	if c.Display.Theme == "" {
		c.Display.Theme = d.Display.Theme
	}
	return c
}

func cloneConfig(c types.Config) types.Config {
	cals := make([]types.Calendar, len(c.Calendars))
	copy(cals, c.Calendars)
	c.Calendars = cals
	return c
}

package types

import (
	"encoding/json"
	"time"
)

type Calendar struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
	URL   string `json:"url"`
}

type Weather struct {
	Enabled   bool    `json:"enabled"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Units     string  `json:"units"`
	Timezone  string  `json:"timezone"`
	Location  string  `json:"location"`
}

type Tide struct {
	Enabled   bool    `json:"enabled"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Units     string  `json:"units"`
	Timezone  string  `json:"timezone"`
	Location  string  `json:"location"`
}

type Display struct {
	DefaultView            string `json:"defaultView"`
	CalendarRefreshSeconds int    `json:"calendarRefreshSeconds"`
	WeatherRefreshSeconds  int    `json:"weatherRefreshSeconds"`
	TideRefreshSeconds     int    `json:"tideRefreshSeconds"`
	BaseballRefreshSeconds int    `json:"baseballRefreshSeconds"`
	Theme                  string `json:"theme"`
	Mode                   string `json:"mode"`
	CalendarEnabled        bool   `json:"calendarEnabled"`
	ClockEnabled           bool   `json:"clockEnabled"`
}

type SnowDay struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url"`
}

type Baseball struct {
	Enabled  bool   `json:"enabled"`
	TeamID   int    `json:"teamId"`
	TeamName string `json:"teamName"`
	TeamAbbr string `json:"teamAbbr"`
}

type Config struct {
	Calendars []Calendar `json:"calendars"`
	Weather   Weather    `json:"weather"`
	Tide      Tide       `json:"tide"`
	SnowDay   SnowDay    `json:"snowDay"`
	Baseball  Baseball   `json:"baseball"`
	Display   Display    `json:"display"`
}

type Event struct {
	ID            string    `json:"id"`
	CalendarID    string    `json:"calendarId"`
	CalendarName  string    `json:"calendarName"`
	CalendarColor string    `json:"calendarColor"`
	Title         string    `json:"title"`
	Start         time.Time `json:"start"`
	End           time.Time `json:"end"`
	AllDay        bool      `json:"allDay"`
	Location      string    `json:"location,omitempty"`
	Description   string    `json:"description,omitempty"`
}

// MarshalJSON emits Start/End as date-only strings ("YYYY-MM-DD") for all-day
// events so browsers don't timezone-shift them. Timed events use RFC3339.
func (e Event) MarshalJSON() ([]byte, error) {
	type alias Event
	startFmt, endFmt := time.RFC3339, time.RFC3339
	if e.AllDay {
		startFmt, endFmt = "2006-01-02", "2006-01-02"
	}
	return json.Marshal(&struct {
		alias
		Start string `json:"start"`
		End   string `json:"end"`
	}{
		alias: alias(e),
		Start: e.Start.Format(startFmt),
		End:   e.End.Format(endFmt),
	})
}

type WeatherCurrent struct {
	Time          time.Time `json:"time"`
	TemperatureC  float64   `json:"temperatureC"`
	ApparentC     float64   `json:"apparentC"`
	Humidity      int       `json:"humidity"`
	WindSpeed     float64   `json:"windSpeed"`
	WeatherCode   int       `json:"weatherCode"`
	IsDay         bool      `json:"isDay"`
	Precipitation float64   `json:"precipitation"`
}

type WeatherDaily struct {
	Date         string  `json:"date"`
	MaxC         float64 `json:"maxC"`
	MinC         float64 `json:"minC"`
	WeatherCode  int     `json:"weatherCode"`
	Sunrise      string  `json:"sunrise"`
	Sunset       string  `json:"sunset"`
	PrecipMM     float64 `json:"precipMM"`
	WindSpeedMax float64 `json:"windSpeedMax"`
}

type WeatherSnapshot struct {
	UpdatedAt time.Time      `json:"updatedAt"`
	Units     string         `json:"units"`
	Timezone  string         `json:"timezone"`
	Current   WeatherCurrent `json:"current"`
	Daily     []WeatherDaily `json:"daily"`
}

type TideEvent struct {
	Time         time.Time `json:"time"`
	Type         string    `json:"type"`
	HeightMeters float64   `json:"heightMeters"`
}

type TideSnapshot struct {
	UpdatedAt     time.Time   `json:"updatedAt"`
	Units         string      `json:"units"`
	Timezone      string      `json:"timezone"`
	CurrentMeters float64     `json:"currentMeters"`
	Events        []TideEvent `json:"events"`
}

type SnowDaySnapshot struct {
	UpdatedAt   time.Time `json:"updatedAt"`
	URL         string    `json:"url"`
	Location    string    `json:"location"`
	RegionName  string    `json:"regionName"`
	MorningTime time.Time `json:"morningTime"`
	Probability int       `json:"probability"`
	Score       int       `json:"score"`
	Category    string    `json:"category"`
}

type BaseballGame struct {
	GameTime      time.Time `json:"gameTime"`
	Opponent      string    `json:"opponent"`
	OpponentAbbr  string    `json:"opponentAbbr"`
	HomeAway      string    `json:"homeAway"`
	Venue         string    `json:"venue,omitempty"`
	Status        string    `json:"status"`
	IsFinal       bool      `json:"isFinal"`
	IsLive        bool      `json:"isLive"`
	TeamScore     int       `json:"teamScore"`
	OpponentScore int       `json:"opponentScore"`
	GameType      string    `json:"gameType"`
	Inning        int       `json:"inning,omitempty"`
	InningHalf    string    `json:"inningHalf,omitempty"`
	Outs          int       `json:"outs,omitempty"`
}

type BaseballSnapshot struct {
	UpdatedAt  time.Time     `json:"updatedAt"`
	TeamID     int           `json:"teamId"`
	TeamName   string        `json:"teamName"`
	TeamAbbr   string        `json:"teamAbbr"`
	LiveGame   *BaseballGame `json:"liveGame"`
	LatestGame *BaseballGame `json:"latestGame"`
	NextGame   *BaseballGame `json:"nextGame"`
}

func DefaultConfig() Config {
	return Config{
		Calendars: []Calendar{},
		Weather: Weather{
			Enabled:   true,
			Latitude:  43.65,
			Longitude: -79.38,
			Units:     "metric",
			Timezone:  "auto",
			Location:  "Toronto, Ontario, Canada",
		},
		Tide: Tide{
			Enabled:  true,
			Units:    "metric",
			Timezone: "auto",
		},
		SnowDay: SnowDay{
			Enabled: true,
		},
		Baseball: Baseball{
			Enabled: true,
		},
		Display: Display{
			DefaultView:            "week",
			CalendarRefreshSeconds: 300,
			WeatherRefreshSeconds:  900,
			TideRefreshSeconds:     3600,
			BaseballRefreshSeconds: 600,
			Theme:                  "default",
			Mode:                   "light",
			CalendarEnabled:        true,
			ClockEnabled:           true,
		},
	}
}

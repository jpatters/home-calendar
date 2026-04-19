package types

import "time"

type Calendar struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
	URL   string `json:"url"`
}

type Weather struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Units     string  `json:"units"`
	Timezone  string  `json:"timezone"`
}

type Display struct {
	DefaultView            string `json:"defaultView"`
	CalendarRefreshSeconds int    `json:"calendarRefreshSeconds"`
	WeatherRefreshSeconds  int    `json:"weatherRefreshSeconds"`
	Theme                  string `json:"theme"`
}

type Config struct {
	Calendars []Calendar `json:"calendars"`
	Weather   Weather    `json:"weather"`
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
	Date        string  `json:"date"`
	MaxC        float64 `json:"maxC"`
	MinC        float64 `json:"minC"`
	WeatherCode int     `json:"weatherCode"`
	Sunrise     string  `json:"sunrise"`
	Sunset      string  `json:"sunset"`
	PrecipMM    float64 `json:"precipMM"`
}

type WeatherSnapshot struct {
	UpdatedAt time.Time      `json:"updatedAt"`
	Units     string         `json:"units"`
	Timezone  string         `json:"timezone"`
	Current   WeatherCurrent `json:"current"`
	Daily     []WeatherDaily `json:"daily"`
}

func DefaultConfig() Config {
	return Config{
		Calendars: []Calendar{},
		Weather: Weather{
			Latitude:  43.65,
			Longitude: -79.38,
			Units:     "metric",
			Timezone:  "auto",
		},
		Display: Display{
			DefaultView:            "week",
			CalendarRefreshSeconds: 300,
			WeatherRefreshSeconds:  900,
			Theme:                  "light",
		},
	}
}

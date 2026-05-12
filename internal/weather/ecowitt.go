package weather

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jpatters/home-calendar/internal/types"
)

// ecowittInterval is how often the fetcher polls the Ecowitt gateway.
// Exposed as a package var so tests can shorten it.
var ecowittInterval = 60 * time.Second

type EcowittReading struct {
	HasOutdoor     bool
	TemperatureC   float64
	Humidity       int
	WindMS         float64
	WindGustMS     float64
	WindDirection  int
	RainRateMMH    float64
	RainEventMM    float64
	RainDailyMM    float64
	RainWeeklyMM   float64
	RainMonthlyMM  float64
	RainYearlyMM   float64
	HasIndoor      bool
	IndoorTempC    float64
	IndoorHumidity int
	PressureHPa    float64
	SolarWM2       float64
}

type ecowittIDValue struct {
	ID   string `json:"id"`
	Val  string `json:"val"`
	Unit string `json:"unit"`
}

type ecowittLiveData struct {
	CommonList []ecowittIDValue `json:"common_list"`
	Rain       []ecowittIDValue `json:"rain"`
	WH25       []ecowittWH25    `json:"wh25"`
}

type ecowittWH25 struct {
	Intemp string `json:"intemp"`
	Unit   string `json:"unit"`
	Inhumi string `json:"inhumi"`
	Abs    string `json:"abs"`
	Rel    string `json:"rel"`
}

func parseEcowittLiveData(body []byte) (EcowittReading, error) {
	var raw ecowittLiveData
	if err := json.Unmarshal(body, &raw); err != nil {
		return EcowittReading{}, fmt.Errorf("ecowitt: parse json: %w", err)
	}
	var r EcowittReading
	// HasOutdoor only flips when one of the merge-consumed fields parses
	// successfully (temp/humidity/wind speed). Direction-only or "--"-only
	// responses must not trigger a zero-overwrite of the merged snapshot.
	for _, item := range raw.CommonList {
		switch strings.ToLower(item.ID) {
		case "0x02":
			if n, ok := parseTempC(item.Val, item.Unit); ok {
				r.TemperatureC = n
				r.HasOutdoor = true
			}
		case "0x07":
			if n, ok := parsePercentOK(item.Val); ok {
				r.Humidity = n
				r.HasOutdoor = true
			}
		case "0x0a":
			if n, ok := parseIntOK(item.Val); ok {
				r.WindDirection = n
			}
		case "0x0b":
			if n, ok := parseWindMSOK(item.Val); ok {
				r.WindMS = n
				r.HasOutdoor = true
			}
		case "0x0c":
			if n, ok := parseWindMSOK(item.Val); ok {
				r.WindGustMS = n
			}
		case "0x15":
			if n, _, ok := parseFloatWithUnitOK(item.Val); ok {
				r.SolarWM2 = n
			}
		}
	}
	for _, item := range raw.Rain {
		switch strings.ToLower(item.ID) {
		case "0x0e":
			r.RainRateMMH = parseRainMM(item.Val)
		case "0x0d":
			r.RainEventMM = parseRainMM(item.Val)
		case "0x10":
			r.RainDailyMM = parseRainMM(item.Val)
		case "0x11":
			r.RainWeeklyMM = parseRainMM(item.Val)
		case "0x12":
			r.RainMonthlyMM = parseRainMM(item.Val)
		case "0x13":
			r.RainYearlyMM = parseRainMM(item.Val)
		}
	}
	for _, w := range raw.WH25 {
		if n, ok := parseTempC(w.Intemp, w.Unit); ok {
			r.IndoorTempC = n
			r.HasIndoor = true
		}
		if n, ok := parsePercentOK(w.Inhumi); ok {
			r.IndoorHumidity = n
			r.HasIndoor = true
		}
		r.PressureHPa = parsePressureHPa(w.Abs)
		if r.PressureHPa > 0 {
			r.HasIndoor = true
		}
	}
	return r, nil
}

func parseFloatWithUnitOK(s string) (float64, string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, "", false
	}
	i := 0
	for i < len(s) {
		c := s[i]
		if (c >= '0' && c <= '9') || c == '.' || c == '-' || c == '+' {
			i++
			continue
		}
		break
	}
	num := strings.TrimSpace(s[:i])
	unit := strings.TrimSpace(s[i:])
	if num == "" || num == "-" || num == "+" || num == "." {
		return 0, unit, false
	}
	f, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0, unit, false
	}
	return f, unit, true
}

func parsePercentOK(s string) (int, bool) {
	n, _, ok := parseFloatWithUnitOK(s)
	if !ok {
		return 0, false
	}
	return int(n), true
}

func parseIntOK(s string) (int, bool) {
	n, _, ok := parseFloatWithUnitOK(s)
	if !ok {
		return 0, false
	}
	return int(n), true
}

func parseTempC(val, unit string) (float64, bool) {
	n, suffix, ok := parseFloatWithUnitOK(val)
	if !ok {
		return 0, false
	}
	u := strings.ToUpper(strings.TrimSpace(unit))
	if u == "" {
		u = strings.ToUpper(strings.TrimSpace(suffix))
	}
	if u == "F" {
		return (n - 32) * 5 / 9, true
	}
	return n, true
}

func parseWindMSOK(val string) (float64, bool) {
	n, unit, ok := parseFloatWithUnitOK(val)
	if !ok {
		return 0, false
	}
	switch strings.ToLower(unit) {
	case "mph":
		return n * 0.44704, true
	case "km/h", "kmh", "kph":
		return n / 3.6, true
	default:
		return n, true
	}
}

func parseRainMM(val string) float64 {
	n, unit, ok := parseFloatWithUnitOK(val)
	if !ok {
		return 0
	}
	if strings.HasPrefix(strings.ToLower(unit), "in") {
		return n * 25.4
	}
	return n
}

func parsePressureHPa(val string) float64 {
	n, unit, ok := parseFloatWithUnitOK(val)
	if !ok {
		return 0
	}
	u := strings.TrimSpace(unit)
	switch {
	case strings.EqualFold(u, "inHg"):
		return n * 33.8639
	case strings.EqualFold(u, "mmHg"):
		return n * 1.33322
	case strings.EqualFold(u, "kPa"):
		return n * 10
	default: // hPa or unknown — assume already in hPa
		return n
	}
}

// mergeEcowitt overlays a live Ecowitt reading onto the Open-Meteo snapshot.
// It mutates and returns the supplied snapshot. Current temperature, humidity,
// and wind speed are overridden when outdoor sensors are present. Weather
// code, apparent temp, sunrise/sunset, and daily forecast remain untouched.
// Current.Precipitation is intentionally NOT overlaid -- Open-Meteo's value
// is "precipitation amount over the last hour" (mm), while Ecowitt's rain
// rate is mm/hour -- different semantics. The live rain rate is surfaced
// instead via Station.RainRate.
// The Station block carries the full detail set even when only indoor
// sensors are present, so the modal can still show pressure etc.
func mergeEcowitt(s *types.WeatherSnapshot, r EcowittReading, updatedAt time.Time, units string) *types.WeatherSnapshot {
	if s == nil {
		return nil
	}
	if !r.HasOutdoor && !r.HasIndoor {
		return s
	}
	imperial := units == "imperial"
	if r.HasOutdoor {
		if imperial {
			s.Current.TemperatureC = celsiusToFahrenheit(r.TemperatureC)
			s.Current.WindSpeed = msToMph(r.WindMS)
		} else {
			s.Current.TemperatureC = r.TemperatureC
			s.Current.WindSpeed = msToKmh(r.WindMS)
		}
		s.Current.Humidity = r.Humidity
	}
	station := &types.WeatherStation{
		UpdatedAt:      updatedAt,
		HasOutdoor:     r.HasOutdoor,
		HasIndoor:      r.HasIndoor,
		IndoorHumidity: r.IndoorHumidity,
		PressureHPa:    r.PressureHPa,
		WindDirection:  r.WindDirection,
		SolarWM2:       r.SolarWM2,
	}
	if imperial {
		station.IndoorTempC = celsiusToFahrenheit(r.IndoorTempC)
		station.WindGust = msToMph(r.WindGustMS)
		station.RainRate = mmToInches(r.RainRateMMH)
		station.RainEvent = mmToInches(r.RainEventMM)
		station.RainDaily = mmToInches(r.RainDailyMM)
		station.RainWeekly = mmToInches(r.RainWeeklyMM)
		station.RainMonthly = mmToInches(r.RainMonthlyMM)
		station.RainYearly = mmToInches(r.RainYearlyMM)
	} else {
		station.IndoorTempC = r.IndoorTempC
		station.WindGust = msToKmh(r.WindGustMS)
		station.RainRate = r.RainRateMMH
		station.RainEvent = r.RainEventMM
		station.RainDaily = r.RainDailyMM
		station.RainWeekly = r.RainWeeklyMM
		station.RainMonthly = r.RainMonthlyMM
		station.RainYearly = r.RainYearlyMM
	}
	s.Station = station
	return s
}

func celsiusToFahrenheit(c float64) float64 { return c*9/5 + 32 }
func msToKmh(v float64) float64             { return v * 3.6 }
func msToMph(v float64) float64             { return v / 0.44704 }
func mmToInches(v float64) float64          { return v / 25.4 }

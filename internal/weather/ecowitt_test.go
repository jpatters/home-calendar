package weather

import (
	"math"
	"testing"
)

func approxEqual(t *testing.T, name string, got, want, tolerance float64) {
	t.Helper()
	if math.Abs(got-want) > tolerance {
		t.Errorf("%s = %v, want %v (±%v)", name, got, want, tolerance)
	}
}

const fullSampleBody = `{
  "common_list": [
    {"id": "0x02", "val": "6.3", "unit": "C"},
    {"id": "0x07", "val": "88%"},
    {"id": "0x0A", "val": "207"},
    {"id": "0x0B", "val": "3.2 m/s"},
    {"id": "0x0C", "val": "4.1 m/s"},
    {"id": "0x15", "val": "13.40 w/m2"}
  ],
  "wh25": [
    {"intemp": "15.2", "unit": "C", "inhumi": "55%", "abs": "1012.4 hPa", "rel": "1012.4 hPa"}
  ],
  "rain": [
    {"id": "0x0D", "val": "0.8 mm"},
    {"id": "0x0E", "val": "0.4 mm/Hr"},
    {"id": "0x10", "val": "1.3 mm"},
    {"id": "0x11", "val": "3.6 mm"},
    {"id": "0x12", "val": "12.0 mm"},
    {"id": "0x13", "val": "98.0 mm"}
  ]
}`

func TestParseLiveData_OutdoorReadings(t *testing.T) {
	r, err := parseEcowittLiveData([]byte(fullSampleBody))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !r.HasOutdoor {
		t.Fatalf("HasOutdoor = false, want true")
	}
	approxEqual(t, "TemperatureC", r.TemperatureC, 6.3, 0.0001)
	if r.Humidity != 88 {
		t.Errorf("Humidity = %d, want 88", r.Humidity)
	}
	if r.WindDirection != 207 {
		t.Errorf("WindDirection = %d, want 207", r.WindDirection)
	}
	approxEqual(t, "WindMS", r.WindMS, 3.2, 0.0001)
	approxEqual(t, "WindGustMS", r.WindGustMS, 4.1, 0.0001)
	approxEqual(t, "SolarWM2", r.SolarWM2, 13.40, 0.0001)
}

func TestParseLiveData_StationDetails(t *testing.T) {
	r, err := parseEcowittLiveData([]byte(fullSampleBody))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !r.HasIndoor {
		t.Fatalf("HasIndoor = false, want true")
	}
	approxEqual(t, "IndoorTempC", r.IndoorTempC, 15.2, 0.0001)
	if r.IndoorHumidity != 55 {
		t.Errorf("IndoorHumidity = %d, want 55", r.IndoorHumidity)
	}
	approxEqual(t, "PressureHPa", r.PressureHPa, 1012.4, 0.0001)
	approxEqual(t, "RainRateMMH", r.RainRateMMH, 0.4, 0.0001)
	approxEqual(t, "RainEventMM", r.RainEventMM, 0.8, 0.0001)
	approxEqual(t, "RainDailyMM", r.RainDailyMM, 1.3, 0.0001)
	approxEqual(t, "RainWeeklyMM", r.RainWeeklyMM, 3.6, 0.0001)
	approxEqual(t, "RainMonthlyMM", r.RainMonthlyMM, 12.0, 0.0001)
	approxEqual(t, "RainYearlyMM", r.RainYearlyMM, 98.0, 0.0001)
}

func TestParseLiveData_FahrenheitTempConvertsToCelsius(t *testing.T) {
	body := `{
  "common_list": [
    {"id": "0x02", "val": "50.0", "unit": "F"},
    {"id": "0x07", "val": "50%"},
    {"id": "0x0B", "val": "0 m/s"}
  ]
}`
	r, err := parseEcowittLiveData([]byte(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	approxEqual(t, "TemperatureC", r.TemperatureC, 10.0, 0.0001)
}

func TestParseLiveData_MphWindConvertsToMetresPerSecond(t *testing.T) {
	body := `{
  "common_list": [
    {"id": "0x02", "val": "10.0", "unit": "C"},
    {"id": "0x07", "val": "50%"},
    {"id": "0x0B", "val": "10 mph"},
    {"id": "0x0C", "val": "20 mph"}
  ]
}`
	r, err := parseEcowittLiveData([]byte(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	approxEqual(t, "WindMS", r.WindMS, 4.4704, 0.001)
	approxEqual(t, "WindGustMS", r.WindGustMS, 8.9408, 0.001)
}

func TestParseLiveData_KmhWindConvertsToMetresPerSecond(t *testing.T) {
	body := `{
  "common_list": [
    {"id": "0x02", "val": "10.0", "unit": "C"},
    {"id": "0x07", "val": "50%"},
    {"id": "0x0B", "val": "36 km/h"}
  ]
}`
	r, err := parseEcowittLiveData([]byte(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	approxEqual(t, "WindMS", r.WindMS, 10.0, 0.001)
}

func TestParseLiveData_RainInchesPerHourConvertsToMm(t *testing.T) {
	body := `{
  "common_list": [
    {"id": "0x02", "val": "10.0", "unit": "C"},
    {"id": "0x07", "val": "50%"}
  ],
  "rain": [
    {"id": "0x0E", "val": "0.1 in/Hr"},
    {"id": "0x10", "val": "0.5 in"}
  ]
}`
	r, err := parseEcowittLiveData([]byte(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	approxEqual(t, "RainRateMMH", r.RainRateMMH, 2.54, 0.001)
	approxEqual(t, "RainDailyMM", r.RainDailyMM, 12.7, 0.001)
}

func TestParseLiveData_MillimetersOfMercuryConvertsToHpa(t *testing.T) {
	// Some Ecowitt firmware (observed on a real GW1100) reports pressure in
	// mmHg rather than hPa or inHg. Standard sea-level is ~760 mmHg / 1013 hPa.
	body := `{
  "wh25": [
    {"intemp": "20.0", "unit": "C", "inhumi": "50%", "abs": "755.25 mmHg", "rel": "755.25 mmHg"}
  ]
}`
	r, err := parseEcowittLiveData([]byte(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// 755.25 mmHg * 1.33322 = ~1006.91 hPa
	approxEqual(t, "PressureHPa", r.PressureHPa, 1006.91, 0.05)
}

func TestParseLiveData_InchOfMercuryConvertsToHpa(t *testing.T) {
	body := `{
  "wh25": [
    {"intemp": "70.0", "unit": "F", "inhumi": "55%", "abs": "29.92 inHg", "rel": "29.92 inHg"}
  ]
}`
	r, err := parseEcowittLiveData([]byte(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !r.HasIndoor {
		t.Fatalf("HasIndoor = false, want true")
	}
	approxEqual(t, "IndoorTempC", r.IndoorTempC, 21.111, 0.01)
	approxEqual(t, "PressureHPa", r.PressureHPa, 1013.21, 0.05)
}

func TestParseLiveData_MissingOutdoorSensor(t *testing.T) {
	body := `{
  "wh25": [
    {"intemp": "20.0", "unit": "C", "inhumi": "40%", "abs": "1000.0 hPa", "rel": "1000.0 hPa"}
  ]
}`
	r, err := parseEcowittLiveData([]byte(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if r.HasOutdoor {
		t.Errorf("HasOutdoor = true, want false")
	}
	if !r.HasIndoor {
		t.Errorf("HasIndoor = false, want true")
	}
}

func TestParseLiveData_PartialStationDetails(t *testing.T) {
	body := `{
  "common_list": [
    {"id": "0x02", "val": "10.0", "unit": "C"},
    {"id": "0x07", "val": "50%"},
    {"id": "0x0B", "val": "1.0 m/s"}
  ],
  "rain": [
    {"id": "0x0E", "val": "0.0 mm/Hr"}
  ]
}`
	r, err := parseEcowittLiveData([]byte(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !r.HasOutdoor {
		t.Errorf("HasOutdoor = false, want true")
	}
	if r.HasIndoor {
		t.Errorf("HasIndoor = true, want false (no wh25 block)")
	}
	if r.PressureHPa != 0 {
		t.Errorf("PressureHPa = %v, want 0 (no wh25 block)", r.PressureHPa)
	}
	if r.RainDailyMM != 0 {
		t.Errorf("RainDailyMM = %v, want 0 (not provided)", r.RainDailyMM)
	}
}

func TestParseLiveData_GarbageBody_ReturnsError(t *testing.T) {
	if _, err := parseEcowittLiveData([]byte("<html>not json</html>")); err == nil {
		t.Errorf("expected error for non-JSON body, got nil")
	}
}

func TestParseLiveData_DirectionOnlyDoesNotFlipHasOutdoor(t *testing.T) {
	// A failure mode where only wind direction lands but no temp/humidity/wind
	// speed. We must not flip HasOutdoor and trigger a zero-overwrite of
	// Current.TemperatureC.
	body := `{
  "common_list": [
    {"id": "0x0A", "val": "180"},
    {"id": "0x15", "val": "0.0 w/m2"}
  ]
}`
	r, err := parseEcowittLiveData([]byte(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if r.HasOutdoor {
		t.Errorf("HasOutdoor = true with only direction + solar, want false")
	}
}

func TestParseLiveData_AllDashesDoesNotFlipHasOutdoor(t *testing.T) {
	// Ecowitt emits "--" when a sensor is offline. If EVERY outdoor reading
	// is "--", we must not flip HasOutdoor — that would zero-overwrite the
	// merged snapshot's temperature and humidity.
	body := `{
  "common_list": [
    {"id": "0x02", "val": "--", "unit": "C"},
    {"id": "0x07", "val": "--"},
    {"id": "0x0B", "val": "--"}
  ]
}`
	r, err := parseEcowittLiveData([]byte(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if r.HasOutdoor {
		t.Errorf("HasOutdoor = true with all dashes, want false")
	}
}

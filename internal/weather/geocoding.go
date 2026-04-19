package weather

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

const DefaultGeocodingURL = "https://geocoding-api.open-meteo.com/v1/search"

type GeoResult struct {
	Name      string  `json:"name"`
	Admin1    string  `json:"admin1,omitempty"`
	Country   string  `json:"country,omitempty"`
	Timezone  string  `json:"timezone,omitempty"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type geocodingResponse struct {
	Results []GeoResult `json:"results"`
}

// Search queries the Open-Meteo geocoding API for a place name and returns
// up to 5 matching locations.
func Search(ctx context.Context, client *http.Client, baseURL, query string) ([]GeoResult, error) {
	if query == "" {
		return nil, errors.New("weather: empty query")
	}
	if client == nil {
		client = http.DefaultClient
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("weather: parse geocoding URL: %w", err)
	}
	q := u.Query()
	q.Set("name", query)
	q.Set("count", "5")
	q.Set("language", "en")
	q.Set("format", "json")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("weather: geocoding http %d", resp.StatusCode)
	}
	var body geocodingResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("weather: decode geocoding response: %w", err)
	}
	if body.Results == nil {
		return []GeoResult{}, nil
	}
	return body.Results, nil
}

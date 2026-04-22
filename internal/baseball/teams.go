package baseball

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type TeamResult struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	TeamName     string `json:"teamName"`
	Abbreviation string `json:"abbreviation"`
	LocationName string `json:"locationName"`
}

type teamsResponse struct {
	Teams []TeamResult `json:"teams"`
}

// SearchTeams fetches the full MLB team list and returns entries matching
// the query string (case-insensitive substring against name, teamName,
// abbreviation, and locationName). An empty query returns all teams.
func SearchTeams(ctx context.Context, client *http.Client, baseURL, query string) ([]TeamResult, error) {
	if client == nil {
		client = http.DefaultClient
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("baseball: parse URL: %w", err)
	}
	q := u.Query()
	q.Set("sportId", "1")
	q.Set("activeStatus", "Y")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("baseball: MLB teams api http %d", resp.StatusCode)
	}

	var body teamsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("baseball: decode teams response: %w", err)
	}

	needle := strings.ToLower(strings.TrimSpace(query))
	out := []TeamResult{}
	for _, t := range body.Teams {
		if needle == "" || matches(t, needle) {
			out = append(out, t)
		}
	}
	return out, nil
}

func matches(t TeamResult, needle string) bool {
	return strings.Contains(strings.ToLower(t.Name), needle) ||
		strings.Contains(strings.ToLower(t.TeamName), needle) ||
		strings.Contains(strings.ToLower(t.Abbreviation), needle) ||
		strings.Contains(strings.ToLower(t.LocationName), needle)
}

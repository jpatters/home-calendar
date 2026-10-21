package hockey

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

type TeamResult struct {
	Abbrev     string `json:"abbrev"`
	Name       string `json:"name"`
	PlaceName  string `json:"placeName"`
	CommonName string `json:"commonName"`
}

type standingsResponse struct {
	Standings []standingsTeam `json:"standings"`
}

type standingsTeam struct {
	TeamAbbrev     localized `json:"teamAbbrev"`
	TeamName       localized `json:"teamName"`
	PlaceName      localized `json:"placeName"`
	TeamCommonName localized `json:"teamCommonName"`
}

// SearchTeams reads the current standings (the one NHL endpoint that lists
// exactly the active teams) and returns entries matching the query string
// (case-insensitive substring against full name, abbreviation, place name,
// and common name). An empty query returns all teams.
func SearchTeams(ctx context.Context, client *http.Client, baseURL, query string) ([]TeamResult, error) {
	if client == nil {
		client = http.DefaultClient
	}
	var body standingsResponse
	if err := getJSON(ctx, client, strings.TrimRight(baseURL, "/")+"/v1/standings/now", &body); err != nil {
		return nil, fmt.Errorf("hockey: standings: %w", err)
	}

	needle := strings.ToLower(strings.TrimSpace(query))
	out := []TeamResult{}
	for _, s := range body.Standings {
		t := TeamResult{
			Abbrev:     s.TeamAbbrev.Default,
			Name:       s.TeamName.Default,
			PlaceName:  s.PlaceName.Default,
			CommonName: s.TeamCommonName.Default,
		}
		if needle == "" || matches(t, needle) {
			out = append(out, t)
		}
	}
	return out, nil
}

func matches(t TeamResult, needle string) bool {
	return strings.Contains(strings.ToLower(t.Name), needle) ||
		strings.Contains(strings.ToLower(t.Abbrev), needle) ||
		strings.Contains(strings.ToLower(t.PlaceName), needle) ||
		strings.Contains(strings.ToLower(t.CommonName), needle)
}

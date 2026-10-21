package hockey_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jpatters/home-calendar/internal/hockey"
)

func standingsTeam(abbrev, name, place, common string) map[string]any {
	return map[string]any{
		"teamAbbrev":     map[string]any{"default": abbrev},
		"teamName":       map[string]any{"default": name},
		"placeName":      map[string]any{"default": place},
		"teamCommonName": map[string]any{"default": common},
		"conferenceName": "Eastern",
		"divisionName":   "Atlantic",
	}
}

func standingsStub(t *testing.T, teams []map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/standings/now" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"standings": teams})
	}))
}

func leagueTeams() []map[string]any {
	return []map[string]any{
		standingsTeam("TOR", "Toronto Maple Leafs", "Toronto", "Maple Leafs"),
		// The NHL's place names for the New York teams are not substrings of
		// the full names, so they only match through the place-name branch.
		standingsTeam("NYR", "New York Rangers", "NY Rangers", "Rangers"),
		standingsTeam("NYI", "New York Islanders", "NY Islanders", "Islanders"),
		standingsTeam("EDM", "Edmonton Oilers", "Edmonton", "Oilers"),
	}
}

func TestSearchTeamsFiltersByFullName(t *testing.T) {
	srv := standingsStub(t, leagueTeams())
	defer srv.Close()

	got, err := hockey.SearchTeams(context.Background(), srv.Client(), srv.URL, "maple")
	if err != nil {
		t.Fatalf("SearchTeams: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d: %+v", len(got), got)
	}
	if got[0].Abbrev != "TOR" || got[0].Name != "Toronto Maple Leafs" {
		t.Errorf("got = %+v, want Toronto Maple Leafs (TOR)", got[0])
	}
}

func TestSearchTeamsMatchesAbbreviationCaseInsensitively(t *testing.T) {
	srv := standingsStub(t, leagueTeams())
	defer srv.Close()

	got, err := hockey.SearchTeams(context.Background(), srv.Client(), srv.URL, "nyr")
	if err != nil {
		t.Fatalf("SearchTeams: %v", err)
	}
	if len(got) != 1 || got[0].Abbrev != "NYR" {
		t.Errorf("expected 1 Rangers result for 'nyr', got %+v", got)
	}
}

func TestSearchTeamsMatchesPlaceName(t *testing.T) {
	srv := standingsStub(t, leagueTeams())
	defer srv.Close()

	got, err := hockey.SearchTeams(context.Background(), srv.Client(), srv.URL, "ny rangers")
	if err != nil {
		t.Fatalf("SearchTeams: %v", err)
	}
	if len(got) != 1 || got[0].Abbrev != "NYR" {
		t.Errorf("expected the Rangers via place name 'NY Rangers', got %+v", got)
	}
}

func TestSearchTeamsReturnsEmptyForNoMatch(t *testing.T) {
	srv := standingsStub(t, leagueTeams())
	defer srv.Close()

	got, err := hockey.SearchTeams(context.Background(), srv.Client(), srv.URL, "zzz")
	if err != nil {
		t.Fatalf("SearchTeams: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice for no match, got %+v", got)
	}
}

func TestSearchTeamsHandlesUpstreamFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	got, err := hockey.SearchTeams(context.Background(), srv.Client(), srv.URL, "tor")
	if err == nil {
		t.Errorf("expected error on 500, got %+v", got)
	}
}

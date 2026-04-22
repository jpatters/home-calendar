package baseball_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jpatters/home-calendar/internal/baseball"
)

func teamsStub(t *testing.T, teams []map[string]any) *httptest.Server {
	t.Helper()
	body := map[string]any{"teams": teams}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
}

func TestSearchTeamsFiltersByQuery(t *testing.T) {
	teams := []map[string]any{
		{"id": 147, "name": "New York Yankees", "teamName": "Yankees", "abbreviation": "NYY", "locationName": "New York"},
		{"id": 111, "name": "Boston Red Sox", "teamName": "Red Sox", "abbreviation": "BOS", "locationName": "Boston"},
		{"id": 119, "name": "Los Angeles Dodgers", "teamName": "Dodgers", "abbreviation": "LAD", "locationName": "Los Angeles"},
	}
	srv := teamsStub(t, teams)
	defer srv.Close()

	got, err := baseball.SearchTeams(context.Background(), srv.Client(), srv.URL, "yank")
	if err != nil {
		t.Fatalf("SearchTeams: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d: %+v", len(got), got)
	}
	if got[0].ID != 147 || got[0].Abbreviation != "NYY" {
		t.Errorf("got = %+v, want Yankees (147, NYY)", got[0])
	}
}

func TestSearchTeamsMatchesAbbreviationCaseInsensitively(t *testing.T) {
	teams := []map[string]any{
		{"id": 147, "name": "New York Yankees", "teamName": "Yankees", "abbreviation": "NYY", "locationName": "New York"},
		{"id": 121, "name": "New York Mets", "teamName": "Mets", "abbreviation": "NYM", "locationName": "New York"},
	}
	srv := teamsStub(t, teams)
	defer srv.Close()

	got, err := baseball.SearchTeams(context.Background(), srv.Client(), srv.URL, "nym")
	if err != nil {
		t.Fatalf("SearchTeams: %v", err)
	}
	if len(got) != 1 || got[0].ID != 121 {
		t.Errorf("expected 1 Mets result for 'nym', got %+v", got)
	}
}

func TestSearchTeamsReturnsEmptyForNoMatch(t *testing.T) {
	teams := []map[string]any{
		{"id": 147, "name": "New York Yankees", "teamName": "Yankees", "abbreviation": "NYY", "locationName": "New York"},
	}
	srv := teamsStub(t, teams)
	defer srv.Close()

	got, err := baseball.SearchTeams(context.Background(), srv.Client(), srv.URL, "zzz")
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

	got, err := baseball.SearchTeams(context.Background(), srv.Client(), srv.URL, "yank")
	if err == nil {
		t.Errorf("expected error on 500, got %+v", got)
	}
}

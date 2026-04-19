package weather_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jpatters/home-calendar/internal/weather"
)

const torontoResponse = `{
  "results": [
    {
      "id": 6167865,
      "name": "Toronto",
      "latitude": 43.70011,
      "longitude": -79.4163,
      "country": "Canada",
      "admin1": "Ontario",
      "timezone": "America/Toronto"
    },
    {
      "id": 5130683,
      "name": "Toronto",
      "latitude": 40.46424,
      "longitude": -80.60089,
      "country": "United States",
      "admin1": "Ohio",
      "timezone": "America/New_York"
    }
  ],
  "generationtime_ms": 0.5
}`

const emptyResponse = `{"generationtime_ms": 0.2}`

func TestSearchReturnsMatchingLocations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("name"); got != "toronto" {
			t.Fatalf("expected name=toronto, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(torontoResponse))
	}))
	defer server.Close()

	results, err := weather.Search(context.Background(), server.Client(), server.URL, "toronto")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Name != "Toronto" || results[0].Admin1 != "Ontario" || results[0].Country != "Canada" {
		t.Errorf("first result fields wrong: %+v", results[0])
	}
	if results[0].Timezone != "America/Toronto" {
		t.Errorf("expected timezone America/Toronto, got %q", results[0].Timezone)
	}
	if results[0].Latitude < 43 || results[0].Latitude > 44 {
		t.Errorf("expected latitude near 43.7, got %v", results[0].Latitude)
	}
	if results[1].Country != "United States" {
		t.Errorf("expected second result in United States, got %q", results[1].Country)
	}
}

func TestSearchReturnsEmptyWhenNoResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(emptyResponse))
	}))
	defer server.Close()

	results, err := weather.Search(context.Background(), server.Client(), server.URL, "zzzzzzzz")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestSearchReturnsErrorOnUpstreamFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := weather.Search(context.Background(), server.Client(), server.URL, "anywhere")
	if err == nil {
		t.Fatal("expected error on upstream 500, got nil")
	}
}

func TestSearchRejectsEmptyQuery(t *testing.T) {
	_, err := weather.Search(context.Background(), http.DefaultClient, "http://unused", "")
	if err == nil {
		t.Fatal("expected error for empty query, got nil")
	}
}

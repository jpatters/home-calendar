package agent_test

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/jpatters/home-calendar/internal/agent"
)

// The fixture lives with the display's tests (scene.test.ts) and is checked by
// both, so the outfit Ziggy describes always matches the one drawn on screen.
const outfitFixture = "../../web/src/display/agent/outfits.fixture.json"

type outfitCase struct {
	Name       string `json:"name"`
	At         string `json:"at"`
	QuietStart string `json:"quietStart"`
	QuietEnd   string `json:"quietEnd"`
	Outfit     string `json:"outfit"`
}

func outfitCases(t *testing.T) []outfitCase {
	t.Helper()
	raw, err := os.ReadFile(outfitFixture)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var cases []outfitCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return cases
}

func TestOutfitMatchesTheDisplaysRules(t *testing.T) {
	for _, tc := range outfitCases(t) {
		t.Run(tc.Name, func(t *testing.T) {
			at, err := time.ParseInLocation("2006-01-02T15:04", tc.At, time.Local)
			if err != nil {
				t.Fatalf("parse %q: %v", tc.At, err)
			}
			start, end := tc.QuietStart, tc.QuietEnd
			if start == "" {
				start, end = "22:00", "07:00"
			}
			if got := agent.OutfitAt(at, start, end); got != tc.Outfit {
				t.Errorf("OutfitAt(%s) = %q, want %q", tc.At, got, tc.Outfit)
			}
		})
	}
}

func TestEveryOutfitHasADescription(t *testing.T) {
	seen := map[string]bool{}
	for _, tc := range outfitCases(t) {
		if seen[tc.Outfit] {
			continue
		}
		seen[tc.Outfit] = true
		if agent.DescribeOutfit(tc.Outfit) == "" {
			t.Errorf("outfit %q has no description", tc.Outfit)
		}
	}
}

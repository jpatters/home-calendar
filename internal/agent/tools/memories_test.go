package tools_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpatters/home-calendar/internal/agent/tools"
	"github.com/jpatters/home-calendar/internal/memories"
)

func memoryStore(t *testing.T) *memories.Store {
	t.Helper()
	s, err := memories.Open(filepath.Join(t.TempDir(), "memories.json"))
	if err != nil {
		t.Fatalf("memories.Open: %v", err)
	}
	return s
}

func memoryDeps(t *testing.T) (tools.Deps, *memories.Store) {
	t.Helper()
	d := baseDeps(t)
	store := memoryStore(t)
	d.Memories = store
	return d, store
}

func storedTexts(s *memories.Store) []string {
	var out []string
	for _, m := range s.List() {
		out = append(out, m.Text)
	}
	return out
}

func TestRememberSavesTheFactAndReportsItsID(t *testing.T) {
	d, store := memoryDeps(t)

	out := mustCall(t, d, "remember", `{"text":"Emma has soccer on Tuesdays."}`)

	got := store.List()
	if len(got) != 1 || got[0].Text != "Emma has soccer on Tuesdays." {
		t.Fatalf("stored = %v, want the fact", storedTexts(store))
	}
	if !strings.Contains(out, got[0].ID) {
		t.Errorf("result %s doesn't report the new memory's id %q", out, got[0].ID)
	}
}

func TestUpdateMemoryReplacesTheText(t *testing.T) {
	d, store := memoryDeps(t)
	m, _ := store.Add("Jordan likes the pool at 29 degrees.")

	mustCall(t, d, "update_memory", `{"id":"`+m.ID+`","text":"Jordan likes the pool at 30 degrees."}`)

	if got := storedTexts(store); len(got) != 1 || got[0] != "Jordan likes the pool at 30 degrees." {
		t.Errorf("stored = %q, want the updated text", got)
	}
}

func TestForgetRemovesTheMemory(t *testing.T) {
	d, store := memoryDeps(t)
	m, _ := store.Add("Old fact.")
	store.Add("Kept fact.")

	mustCall(t, d, "forget", `{"id":"`+m.ID+`"}`)

	if got := storedTexts(store); len(got) != 1 || got[0] != "Kept fact." {
		t.Errorf("stored = %q, want only the kept fact", got)
	}
}

func TestMemoryToolsExplainProblemsInPlainWords(t *testing.T) {
	d, store := memoryDeps(t)
	for i := 0; i < memories.MaxMemories; i++ {
		store.Add("fact")
	}

	cases := map[string]string{
		"forgetting an unknown id":   `{"id":"nope"}`,
		"updating an unknown id":     `{"id":"nope","text":"x"}`,
		"remembering when full":      `{"text":"one more"}`,
		"remembering nothing":        `{"text":"  "}`,
		"remembering something long": `{"text":"` + strings.Repeat("a", memories.MaxTextLen+1) + `"}`,
	}
	tool := map[string]string{
		"forgetting an unknown id":   "forget",
		"updating an unknown id":     "update_memory",
		"remembering when full":      "remember",
		"remembering nothing":        "remember",
		"remembering something long": "remember",
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			if err := mustFail(t, d, tool[name], args); strings.TrimSpace(err.Error()) == "" {
				t.Errorf("empty error")
			}
		})
	}
}

func TestMemoryToolsAreLeftOutWithoutAStore(t *testing.T) {
	for _, tool := range tools.All(baseDeps(t)) {
		switch tool.Name() {
		case "remember", "update_memory", "forget":
			t.Errorf("%s offered without a memory store", tool.Name())
		}
	}
}

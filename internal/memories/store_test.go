package memories_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpatters/home-calendar/internal/memories"
)

func open(t *testing.T, dir string) *memories.Store {
	t.Helper()
	s, err := memories.Open(filepath.Join(dir, "memories.json"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return s
}

func texts(s *memories.Store) []string {
	var out []string
	for _, m := range s.List() {
		out = append(out, m.Text)
	}
	return out
}

func TestPathForSitsBesideTheConfigFile(t *testing.T) {
	if got := memories.PathFor("/data/config.json"); got != "/data/memories.json" {
		t.Errorf("PathFor = %q, want /data/memories.json", got)
	}
}

func TestAMissingFileMeansNoMemories(t *testing.T) {
	if got := open(t, t.TempDir()).List(); len(got) != 0 {
		t.Errorf("List = %v, want none", got)
	}
}

func TestMemoriesSurviveReopeningInTheOrderTheyWereAdded(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	for _, text := range []string{"Emma has soccer on Tuesdays.", "Jordan likes the pool at 29 degrees."} {
		if _, err := s.Add(text); err != nil {
			t.Fatalf("Add(%q): %v", text, err)
		}
	}

	got := texts(open(t, dir))

	if strings.Join(got, "|") != "Emma has soccer on Tuesdays.|Jordan likes the pool at 29 degrees." {
		t.Errorf("after reopen = %q", got)
	}
}

func TestAddTrimsTheTextAndGivesEachMemoryItsOwnID(t *testing.T) {
	s := open(t, t.TempDir())
	a, _ := s.Add("  first  ")
	b, _ := s.Add("second")

	if a.Text != "first" {
		t.Errorf("text = %q, want it trimmed", a.Text)
	}
	if a.ID == "" || a.ID == b.ID {
		t.Errorf("ids = %q, %q, want distinct non-empty ids", a.ID, b.ID)
	}
}

func TestUpdateReplacesOnlyThatMemoryAndPersists(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	keep, _ := s.Add("Emma has soccer on Tuesdays.")
	change, _ := s.Add("Jordan likes the pool at 29 degrees.")

	updated, err := s.Update(change.ID, "Jordan likes the pool at 30 degrees.")
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	if updated.ID != change.ID || updated.Text != "Jordan likes the pool at 30 degrees." {
		t.Errorf("Update returned %+v", updated)
	}
	got := texts(open(t, dir))
	if strings.Join(got, "|") != keep.Text+"|Jordan likes the pool at 30 degrees." {
		t.Errorf("after update = %q", got)
	}
}

func TestDeleteRemovesOnlyThatMemoryAndPersists(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	gone, _ := s.Add("Old fact.")
	s.Add("Kept fact.")

	if err := s.Delete(gone.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if got := texts(open(t, dir)); strings.Join(got, "|") != "Kept fact." {
		t.Errorf("after delete = %q", got)
	}
}

func TestUnknownIDsAreNotFound(t *testing.T) {
	s := open(t, t.TempDir())
	if _, err := s.Update("nope", "text"); !errors.Is(err, memories.ErrNotFound) {
		t.Errorf("Update err = %v, want ErrNotFound", err)
	}
	if err := s.Delete("nope"); !errors.Is(err, memories.ErrNotFound) {
		t.Errorf("Delete err = %v, want ErrNotFound", err)
	}
}

func TestBlankAndOverlongTextIsRejected(t *testing.T) {
	s := open(t, t.TempDir())
	kept, _ := s.Add("Fine.")

	for _, text := range []string{"", "   ", strings.Repeat("a", memories.MaxTextLen+1)} {
		if _, err := s.Add(text); !errors.Is(err, memories.ErrInvalid) {
			t.Errorf("Add(%d chars) err = %v, want ErrInvalid", len(text), err)
		}
		if _, err := s.Update(kept.ID, text); !errors.Is(err, memories.ErrInvalid) {
			t.Errorf("Update(%d chars) err = %v, want ErrInvalid", len(text), err)
		}
	}
	if _, err := s.Add(strings.Repeat("a", memories.MaxTextLen)); err != nil {
		t.Errorf("Add at the limit: %v", err)
	}
	if got := texts(s); got[0] != "Fine." {
		t.Errorf("a rejected update changed the memory: %q", got)
	}
}

func TestAFullStoreRejectsNewMemoriesButStillAllowsChanges(t *testing.T) {
	s := open(t, t.TempDir())
	var first memories.Memory
	for i := 0; i < memories.MaxMemories; i++ {
		m, err := s.Add("fact")
		if err != nil {
			t.Fatalf("Add #%d: %v", i, err)
		}
		if i == 0 {
			first = m
		}
	}

	if _, err := s.Add("one too many"); !errors.Is(err, memories.ErrFull) {
		t.Errorf("Add when full err = %v, want ErrFull", err)
	}
	if _, err := s.Update(first.ID, "changed"); err != nil {
		t.Errorf("Update when full: %v", err)
	}
	if err := s.Delete(first.ID); err != nil {
		t.Errorf("Delete when full: %v", err)
	}
	if _, err := s.Add("room again"); err != nil {
		t.Errorf("Add after deleting: %v", err)
	}
}

func TestSavingKeepsTheFileOwnerOnlyAndLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memories.json")
	if err := os.WriteFile(path, []byte(`{"memories":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s := open(t, dir)

	if _, err := s.Add("Private household fact."); err != nil {
		t.Fatalf("Add: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("memories.json mode = %o, want 600", perm)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "memories.json" {
			t.Errorf("leftover file %q", e.Name())
		}
	}
}

func TestACorruptFileFailsToOpenAndIsLeftAlone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memories.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := memories.Open(path); err == nil {
		t.Fatal("Open err = nil, want the corrupt file reported")
	}
	if raw, _ := os.ReadFile(path); string(raw) != "{not json" {
		t.Errorf("corrupt file was rewritten to %q", raw)
	}
}

func TestListReturnsACopy(t *testing.T) {
	s := open(t, t.TempDir())
	s.Add("Original.")

	s.List()[0].Text = "tampered"

	if got := texts(s); got[0] != "Original." {
		t.Errorf("List exposed the store's slice: %q", got)
	}
}

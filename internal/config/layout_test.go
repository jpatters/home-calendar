package config

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestSetLayoutPersistsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	layout := json.RawMessage(`{"grid":{"root":{"type":"branch"}},"panels":{}}`)
	if _, err := s.SetLayout(layout); err != nil {
		t.Fatalf("SetLayout: %v", err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	assertSameJSON(t, reopened.Get().Layout, layout)
}

func TestSetLayoutNilClearsLayout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := s.SetLayout(json.RawMessage(`{"panels":{}}`)); err != nil {
		t.Fatalf("SetLayout: %v", err)
	}
	if _, err := s.SetLayout(nil); err != nil {
		t.Fatalf("SetLayout(nil): %v", err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if got := reopened.Get().Layout; len(got) != 0 {
		t.Fatalf("expected no layout after clearing, got %s", got)
	}
}

func TestReplaceKeepsStoredLayout(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	stale := s.Get()
	layout := json.RawMessage(`{"panels":{"clock":{}}}`)
	if _, err := s.SetLayout(layout); err != nil {
		t.Fatalf("SetLayout: %v", err)
	}

	stale.Display.DefaultView = "month"
	saved, err := s.Replace(stale)
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}
	assertSameJSON(t, saved.Layout, layout)
	assertSameJSON(t, s.Get().Layout, layout)

	stale.Layout = json.RawMessage(`{"panels":{"other":{}}}`)
	saved, err = s.Replace(stale)
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}
	assertSameJSON(t, saved.Layout, layout)
}

func assertSameJSON(t *testing.T, got, want json.RawMessage) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatalf("got is not JSON (%q): %v", got, err)
	}
	if err := json.Unmarshal(want, &w); err != nil {
		t.Fatalf("want is not JSON: %v", err)
	}
	gb, _ := json.Marshal(g)
	wb, _ := json.Marshal(w)
	if string(gb) != string(wb) {
		t.Fatalf("layout mismatch\n got: %s\nwant: %s", gb, wb)
	}
}

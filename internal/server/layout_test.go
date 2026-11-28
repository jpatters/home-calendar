package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPutLayoutSavesAndBroadcastsConfig(t *testing.T) {
	store := newTestStore(t, nil)
	hub := NewHub()
	c := hub.register()
	defer hub.unregister(c)
	srv := &Server{cfg: store, hub: hub}

	body := `{"grid":{"root":{"type":"branch","data":[]}},"panels":{}}`
	rec := httptest.NewRecorder()
	srv.handlePutLayout(rec, httptest.NewRequest(http.MethodPut, "/api/layout", strings.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}

	getRec := httptest.NewRecorder()
	srv.handleGetConfig(getRec, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	var cfg struct {
		Layout json.RawMessage `json:"layout"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	assertSameJSON(t, cfg.Layout, body)

	select {
	case data := <-c.send:
		f := decodeConfigFrame(t, data)
		assertSameJSON(t, f.Config.Layout, body)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("no frame broadcast after saving layout")
	}
}

type configFrame struct {
	Type   string `json:"type"`
	Config struct {
		Layout json.RawMessage `json:"layout"`
	} `json:"config"`
}

func decodeConfigFrame(t *testing.T, data []byte) configFrame {
	t.Helper()
	var f configFrame
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("decode frame: %v", err)
	}
	if f.Type != "config" {
		t.Fatalf("expected config frame, got %q", f.Type)
	}
	return f
}

func assertSameJSON(t *testing.T, got json.RawMessage, want string) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatalf("got is not JSON (%q): %v", got, err)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("want is not JSON: %v", err)
	}
	gb, _ := json.Marshal(g)
	wb, _ := json.Marshal(w)
	if string(gb) != string(wb) {
		t.Fatalf("layout mismatch\n got: %s\nwant: %s", gb, wb)
	}
}

func TestPutLayoutNullClearsLayout(t *testing.T) {
	store := newTestStore(t, nil)
	hub := NewHub()
	srv := &Server{cfg: store, hub: hub}

	put := func(body string) int {
		rec := httptest.NewRecorder()
		srv.handlePutLayout(rec, httptest.NewRequest(http.MethodPut, "/api/layout", strings.NewReader(body)))
		return rec.Code
	}
	if code := put(`{"panels":{}}`); code != http.StatusOK {
		t.Fatalf("expected 200 saving layout, got %d", code)
	}
	c := hub.register()
	defer hub.unregister(c)
	if code := put(`null`); code != http.StatusOK {
		t.Fatalf("expected 200 clearing layout, got %d", code)
	}

	getRec := httptest.NewRecorder()
	srv.handleGetConfig(getRec, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	var cfg map[string]any
	if err := json.Unmarshal(getRec.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if v, ok := cfg["layout"]; ok && v != nil {
		t.Fatalf("expected layout to be cleared, got %v", v)
	}

	select {
	case data := <-c.send:
		if f := decodeConfigFrame(t, data); len(f.Config.Layout) != 0 {
			t.Fatalf("clearing frame still carried a layout: %s", f.Config.Layout)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("no frame broadcast after clearing layout")
	}
}

func TestPutLayoutRejectsNonObjectBodies(t *testing.T) {
	cases := map[string]string{
		"array":     `[1,2]`,
		"string":    `"layout"`,
		"number":    `42`,
		"malformed": `{not json`,
		"empty":     ``,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			saved := `{"panels":{"clock":{}}}`
			srv := &Server{cfg: newTestStore(t, nil), hub: NewHub()}
			seed := httptest.NewRecorder()
			srv.handlePutLayout(seed, httptest.NewRequest(http.MethodPut, "/api/layout", strings.NewReader(saved)))
			if seed.Code != http.StatusOK {
				t.Fatalf("seeding layout: got %d", seed.Code)
			}

			rec := httptest.NewRecorder()
			srv.handlePutLayout(rec, httptest.NewRequest(http.MethodPut, "/api/layout", strings.NewReader(body)))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d", rec.Code)
			}
			assertSameJSON(t, srv.cfg.Get().Layout, saved)
		})
	}
}

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpatters/home-calendar/internal/config"
	"github.com/jpatters/home-calendar/internal/memories"
	"github.com/jpatters/home-calendar/internal/secrets"
)

type memoryJSON struct {
	ID      string `json:"id"`
	Text    string `json:"text"`
	Created string `json:"created"`
	Updated string `json:"updated"`
}

func memoriesRequest(t *testing.T, method, url, body string) (int, []byte) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, _ := http.NewRequest(method, url, r)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

func listMemories(t *testing.T, base string) []memoryJSON {
	t.Helper()
	status, raw := memoriesRequest(t, http.MethodGet, base+"/api/agent/memories", "")
	if status != http.StatusOK {
		t.Fatalf("GET memories = %d %s", status, raw)
	}
	var out struct {
		Memories []memoryJSON `json:"memories"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return out.Memories
}

func addMemory(t *testing.T, base, text string) memoryJSON {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"text": text})
	status, raw := memoriesRequest(t, http.MethodPost, base+"/api/agent/memories", string(body))
	if status != http.StatusOK {
		t.Fatalf("POST memory = %d %s", status, raw)
	}
	var m memoryJSON
	json.Unmarshal(raw, &m)
	return m
}

func TestMemoriesCanBeAddedEditedAndDeletedFromAdmin(t *testing.T) {
	openai, _ := fakeResponsesAPI(t, "ok")
	app := agentApp(t, true, "sk-test", openai.URL)

	if got := listMemories(t, app.URL); len(got) != 0 {
		t.Fatalf("initial memories = %v, want none", got)
	}
	soccer := addMemory(t, app.URL, "Emma has soccer on Tuesdays.")
	pool := addMemory(t, app.URL, "Jordan likes the pool at 29 degrees.")
	if soccer.ID == "" || soccer.Text != "Emma has soccer on Tuesdays." || soccer.Updated == "" {
		t.Errorf("added memory = %+v", soccer)
	}

	status, raw := memoriesRequest(t, http.MethodPut, app.URL+"/api/agent/memories/"+pool.ID, `{"text":"Jordan likes the pool at 30 degrees."}`)
	if status != http.StatusOK || !strings.Contains(string(raw), "30 degrees") {
		t.Errorf("PUT = %d %s", status, raw)
	}
	if status, raw := memoriesRequest(t, http.MethodDelete, app.URL+"/api/agent/memories/"+soccer.ID, ""); status != http.StatusNoContent {
		t.Errorf("DELETE = %d %s", status, raw)
	}

	got := listMemories(t, app.URL)
	if len(got) != 1 || got[0].ID != pool.ID || got[0].Text != "Jordan likes the pool at 30 degrees." {
		t.Errorf("memories = %+v, want only the edited pool memory", got)
	}
}

func TestAFreshInstallListsNoMemoriesAsAnEmptyList(t *testing.T) {
	openai, _ := fakeResponsesAPI(t, "ok")
	app := agentApp(t, true, "sk-test", openai.URL)

	_, raw := memoriesRequest(t, http.MethodGet, app.URL+"/api/agent/memories", "")

	if strings.TrimSpace(string(raw)) != `{"memories":[]}` {
		t.Errorf("body = %s, want an empty list", raw)
	}
}

func TestMemoryEndpointsRejectBadInputAndUnknownIDs(t *testing.T) {
	openai, _ := fakeResponsesAPI(t, "ok")
	app := agentApp(t, true, "sk-test", openai.URL)
	kept := addMemory(t, app.URL, "Kept.")

	cases := []struct {
		name, method, path, body string
		status                   int
	}{
		{"blank text", http.MethodPost, "/api/agent/memories", `{"text":"  "}`, http.StatusBadRequest},
		{"not JSON", http.MethodPost, "/api/agent/memories", `{nope`, http.StatusBadRequest},
		{"too long", http.MethodPost, "/api/agent/memories", `{"text":"` + strings.Repeat("a", memories.MaxTextLen+1) + `"}`, http.StatusBadRequest},
		{"blank edit", http.MethodPut, "/api/agent/memories/" + kept.ID, `{"text":""}`, http.StatusBadRequest},
		{"edit unknown", http.MethodPut, "/api/agent/memories/nope", `{"text":"x"}`, http.StatusNotFound},
		{"delete unknown", http.MethodDelete, "/api/agent/memories/nope", "", http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, raw := memoriesRequest(t, tc.method, app.URL+tc.path, tc.body)
			if status != tc.status {
				t.Errorf("status = %d %s, want %d", status, raw, tc.status)
			}
			var e struct {
				Error string `json:"error"`
			}
			if json.Unmarshal(raw, &e); strings.TrimSpace(e.Error) == "" {
				t.Errorf("body %s has no error message", raw)
			}
		})
	}
	if got := listMemories(t, app.URL); len(got) != 1 || got[0].Text != "Kept." {
		t.Errorf("rejected requests changed the memories: %+v", got)
	}
}

func TestZiggyIsGivenTheMemoriesAndTheToolsToChangeThem(t *testing.T) {
	openai := newFakeResponses(t, "ok")
	app := agentApp(t, true, "sk-test", openai.URL)
	addMemory(t, app.URL, "Emma has soccer on Tuesdays.")

	postConverse(t, app, `{"conversation_id":"c","query":"when is soccer?"}`)

	openai.mu.Lock()
	defer openai.mu.Unlock()
	if len(openai.instructions) == 0 || !strings.Contains(openai.instructions[0], "Emma has soccer on Tuesdays.") {
		t.Errorf("instructions don't include the memory: %q", openai.instructions)
	}
	tools := strings.Join(openai.tools[0], " ")
	for _, name := range []string{"remember", "update_memory", "forget"} {
		if !strings.Contains(" "+tools+" ", " "+name+" ") {
			t.Errorf("tools %q missing %s", tools, name)
		}
	}
}

func TestAddingToAFullMemoryExplainsWhyInAdmin(t *testing.T) {
	openai, _ := fakeResponsesAPI(t, "ok")
	app := agentApp(t, true, "sk-test", openai.URL)
	for i := 0; i < memories.MaxMemories; i++ {
		addMemory(t, app.URL, "fact")
	}

	status, raw := memoriesRequest(t, http.MethodPost, app.URL+"/api/agent/memories", `{"text":"one more"}`)

	var e struct {
		Error string `json:"error"`
	}
	json.Unmarshal(raw, &e)
	if status != http.StatusBadRequest || !strings.HasPrefix(e.Error, "Memory is full") {
		t.Errorf("POST when full = %d %q, want 400 saying memory is full", status, e.Error)
	}
}

func TestMemoriesNeverReachThePublicConfigOrWebSocket(t *testing.T) {
	openai, _ := fakeResponsesAPI(t, "ok")
	app := agentApp(t, true, "sk-test", openai.URL)
	addMemory(t, app.URL, "A private household fact.")

	resp, err := http.Get(app.URL + "/api/config")
	if err != nil {
		t.Fatalf("GET config: %v", err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !bytes.Contains(raw, []byte(`"agent"`)) {
		t.Fatalf("GET /api/config = %d %s, want the config", resp.StatusCode, raw)
	}
	if bytes.Contains(raw, []byte("private household fact")) {
		t.Errorf("GET /api/config exposes a memory")
	}

	conn, ctx := dialWS(t, app)
	postConverse(t, app, `{"conversation_id":"c","query":"what do you remember?"}`)
	for {
		_, frame, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("read ws before the reply frame: %v", err)
		}
		if bytes.Contains(frame, []byte("private household fact")) {
			t.Errorf("a WebSocket frame exposes a memory: %s", frame)
		}
		if bytes.Contains(frame, []byte(`"talking"`)) {
			break
		}
	}
}

func TestTheServerWontStartWithoutAMemoryStore(t *testing.T) {
	dir := t.TempDir()
	store, err := config.Open(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	sec, err := secrets.Open(filepath.Join(dir, "secrets.json"))
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := New(context.Background(), Options{Config: store, Secrets: sec}); err == nil {
		t.Error("New err = nil, want it to refuse to start without memories")
	}
}

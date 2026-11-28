package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
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
		{"too long", http.MethodPost, "/api/agent/memories", `{"text":"` + strings.Repeat("a", 301) + `"}`, http.StatusBadRequest},
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
	if bytes.Contains(raw, []byte("private household fact")) {
		t.Errorf("GET /api/config exposes a memory")
	}

	conn, ctx := dialWS(t, app)
	_, frame, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read ws: %v", err)
	}
	if bytes.Contains(frame, []byte("private household fact")) {
		t.Errorf("the WebSocket snapshot exposes a memory")
	}
}

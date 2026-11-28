package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/jpatters/home-calendar/internal/config"
	"github.com/jpatters/home-calendar/internal/secrets"
	"github.com/jpatters/home-calendar/internal/types"
)

// fakeResponses emulates POST /responses as a server-sent event stream that
// completes with reply, or answers 400 when reply is empty. It records the
// Authorization header and tool names of every request.
type fakeResponses struct {
	*httptest.Server
	hits  atomic.Int32
	mu    sync.Mutex
	auths []string
	tools [][]string
}

func newFakeResponses(t *testing.T, reply string) *fakeResponses {
	t.Helper()
	f := &fakeResponses{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/responses") {
			http.NotFound(w, r)
			return
		}
		f.hits.Add(1)
		var body struct {
			Tools []struct {
				Type  string `json:"type"`
				Name  string `json:"name"`
				Tools []struct {
					Name string `json:"name"`
				} `json:"tools"`
			} `json:"tools"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		var names []string
		for _, tool := range body.Tools {
			if tool.Type != "namespace" {
				names = append(names, tool.Name)
			}
			for _, inner := range tool.Tools {
				names = append(names, inner.Name)
			}
		}
		f.mu.Lock()
		f.auths = append(f.auths, r.Header.Get("Authorization"))
		f.tools = append(f.tools, names)
		f.mu.Unlock()
		if reply == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"error":{"message":"secret upstream detail","type":"invalid_request_error"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		final := fmt.Sprintf(`{"id":"resp_1","object":"response","created_at":1764200000,"status":"completed","model":"gpt-test","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":%q,"annotations":[]}]}]}`, reply)
		fmt.Fprintf(w, "event: response.created\ndata: %s\n\n", `{"type":"response.created","sequence_number":0,"response":{"id":"resp_1","object":"response","created_at":1764200000,"status":"in_progress","model":"gpt-test","output":[]}}`)
		fmt.Fprintf(w, "event: response.completed\ndata: {\"type\":\"response.completed\",\"sequence_number\":1,\"response\":%s}\n\n", final)
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeResponses) authorizations() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.auths...)
}

// lastTools returns the tool names offered in the latest request.
func (f *fakeResponses) lastTools() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.tools) == 0 {
		return nil
	}
	return f.tools[len(f.tools)-1]
}

// fakeResponsesAPI answers every POST /responses with reply, or with a 400
// when reply is empty.
func fakeResponsesAPI(t *testing.T, reply string) (*httptest.Server, *atomic.Int32) {
	f := newFakeResponses(t, reply)
	return f.Server, &f.hits
}

// agentApp builds the full router via New with every widget disabled (so no
// fetcher reaches the network), the agent enabled or not, OPENAI_API_KEY set
// to apiKey, and OpenAI pointed at openaiURL through the SDK's standard
// environment variable.
func agentApp(t *testing.T, enabled bool, apiKey, openaiURL string) *httptest.Server {
	t.Helper()
	t.Setenv("OPENAI_API_KEY", apiKey)
	t.Setenv("OPENAI_BASE_URL", openaiURL)
	t.Setenv("GOOGLE_SERVICE_ACCOUNT_JSON", "")
	dir := t.TempDir()
	return startApp(t, dir, enabled, "")
}

// startApp runs New over the config and secrets files in dir, as a server
// (re)started on that data directory would. authIssuer, when set, is the
// Sign in with ChatGPT server.
func startApp(t *testing.T, dir string, enabled bool, authIssuer string) *httptest.Server {
	t.Helper()
	configPath := filepath.Join(dir, "config.json")
	store, err := config.Open(configPath)
	if err != nil {
		t.Fatalf("config.Open: %v", err)
	}
	cfg := store.Get()
	cfg.Weather.Enabled = false
	cfg.Tide.Enabled = false
	cfg.SnowDay.Enabled = false
	cfg.Baseball.Enabled = false
	cfg.Hockey.Enabled = false
	cfg.Pool.Enabled = false
	cfg.HotTub.Enabled = false
	cfg.Display.CalendarEnabled = false
	cfg.Agent.Enabled = enabled
	if _, err := store.Replace(cfg); err != nil {
		t.Fatalf("Replace: %v", err)
	}
	sec, err := secrets.Open(secrets.PathFor(configPath))
	if err != nil {
		t.Fatalf("secrets.Open: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s, h, err := New(ctx, Options{Config: store, Secrets: sec, ChatGPTIssuer: authIssuer})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(s.Shutdown)
	app := httptest.NewServer(h)
	t.Cleanup(app.Close)
	return app
}

func postConverse(t *testing.T, app *httptest.Server, body string) (int, map[string]any) {
	t.Helper()
	resp, err := http.Post(app.URL+"/api/agent/converse", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func dialWS(t *testing.T, app *httptest.Server) (*websocket.Conn, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(app.URL, "http")+"/api/ws", nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	t.Cleanup(func() { conn.Close(websocket.StatusNormalClosure, "") })
	return conn, ctx
}

func readFrame(t *testing.T, ctx context.Context, conn *websocket.Conn) Frame {
	t.Helper()
	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read ws: %v", err)
	}
	var f Frame
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("decode frame %s: %v", data, err)
	}
	return f
}

func TestAgentConverseReturnsSpokenReplyForHomeAssistant(t *testing.T) {
	openai, hits := fakeResponsesAPI(t, "It's sunny and minus three.")
	app := agentApp(t, true, "sk-test", openai.URL)

	status, body := postConverse(t, app, `{"conversation_id":"01J8","query":"What's the weather?","language":"en","agent_id":"conversation.webhook","device_id":"kitchen"}`)

	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %v)", status, body)
	}
	if body["output"] != "It's sunny and minus three." {
		t.Errorf("output = %v, want the agent's reply", body["output"])
	}
	if hits.Load() != 1 {
		t.Errorf("OpenAI hits = %d, want 1", hits.Load())
	}
}

// With response streaming enabled (the integration's default), Home Assistant
// sends "stream": true and reads newline-delimited JSON items.
func TestAgentConverseStreamsTheReplyWhenHomeAssistantAsks(t *testing.T) {
	openai, _ := fakeResponsesAPI(t, "It's sunny and minus three.")
	app := agentApp(t, true, "sk-test", openai.URL)

	resp, err := http.Post(app.URL+"/api/agent/converse", "application/json",
		bytes.NewBufferString(`{"conversation_id":"01J8","query":"What's the weather?","stream":true}`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	want := []string{`{"content":"It's sunny and minus three.","type":"item"}`, `{"type":"end"}`}
	if len(lines) != len(want) {
		t.Fatalf("lines = %q, want %q", lines, want)
	}
	for i := range want {
		var got, exp any
		if err := json.Unmarshal([]byte(lines[i]), &got); err != nil {
			t.Fatalf("line %d %q is not JSON: %v", i, lines[i], err)
		}
		_ = json.Unmarshal([]byte(want[i]), &exp)
		if fmt.Sprint(got) != fmt.Sprint(exp) {
			t.Errorf("line %d = %s, want %s", i, lines[i], want[i])
		}
	}
}

func TestAgentConverseStreamsSpokenRefusalsToo(t *testing.T) {
	openai, _ := fakeResponsesAPI(t, "unused")
	app := agentApp(t, false, "sk-test", openai.URL)

	resp, err := http.Post(app.URL+"/api/agent/converse", "application/json",
		bytes.NewBufferString(`{"conversation_id":"c","query":"hello","stream":true}`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if !strings.HasPrefix(string(raw), `{"content":`) || !strings.HasSuffix(strings.TrimSpace(string(raw)), `{"type":"end"}`) {
		t.Errorf("body = %s, want a streamed item then end", raw)
	}
}

// Home Assistant's webhook conversation integration treats any non-200 as a
// failure and speaks nothing, so refusals are 200s with a spoken output.
func TestAgentConverseExplainsWhenAgentDisabled(t *testing.T) {
	openai, hits := fakeResponsesAPI(t, "unused")
	app := agentApp(t, false, "sk-test", openai.URL)

	status, body := postConverse(t, app, `{"conversation_id":"c","query":"hello"}`)

	if status != http.StatusOK {
		t.Errorf("status = %d, want 200", status)
	}
	if s, _ := body["output"].(string); strings.TrimSpace(s) == "" {
		t.Errorf("body = %v, want a spoken output explaining the agent is off", body)
	}
	if hits.Load() != 0 {
		t.Errorf("OpenAI hits = %d, want 0", hits.Load())
	}
}

func TestAgentConverseHonoursAgentBeingDisabledAfterStartup(t *testing.T) {
	openai, hits := fakeResponsesAPI(t, "unused")
	app := agentApp(t, true, "sk-test", openai.URL)
	resp, err := http.Get(app.URL + "/api/config")
	if err != nil {
		t.Fatalf("GET config: %v", err)
	}
	var cfg types.Config
	json.NewDecoder(resp.Body).Decode(&cfg)
	resp.Body.Close()
	cfg.Agent.Enabled = false
	raw, _ := json.Marshal(cfg)
	req, _ := http.NewRequest(http.MethodPut, app.URL+"/api/config", bytes.NewReader(raw))
	if resp, err := http.DefaultClient.Do(req); err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT config: %v %v", err, resp)
	}

	status, body := postConverse(t, app, `{"conversation_id":"c","query":"hello"}`)

	if s, _ := body["output"].(string); status != http.StatusOK || strings.TrimSpace(s) == "" {
		t.Errorf("status %d body %v, want 200 with a spoken explanation", status, body)
	}
	if hits.Load() != 0 {
		t.Errorf("OpenAI hits = %d, want 0", hits.Load())
	}
}

func TestAgentConverseApologisesWhenModelFails(t *testing.T) {
	openai, _ := fakeResponsesAPI(t, "")
	app := agentApp(t, true, "sk-test", openai.URL)
	conn, ctx := dialWS(t, app)
	readFrame(t, ctx, conn) // initial snapshot

	status, body := postConverse(t, app, `{"conversation_id":"c","query":"hello"}`)

	s, _ := body["output"].(string)
	if status != http.StatusOK || strings.TrimSpace(s) == "" {
		t.Errorf("status %d body %v, want 200 with a spoken apology", status, body)
	}
	if strings.Contains(s, "secret upstream detail") {
		t.Errorf("output %q leaks the API error", s)
	}
	for {
		f := readFrame(t, ctx, conn)
		if f.Type != "agent" || f.Agent == nil || f.Agent.State != types.AgentError {
			continue
		}
		if raw, _ := json.Marshal(f); strings.Contains(string(raw), "secret upstream detail") {
			t.Errorf("agent frame %s leaks the API error", raw)
		}
		break
	}
}

func TestAgentOffersOnlyCalendarReadsWithoutGoogleCredentials(t *testing.T) {
	openai := newFakeResponses(t, "ok")
	app := agentApp(t, true, "sk-test", openai.URL)

	if status, body := postConverse(t, app, `{"conversation_id":"c","query":"add lunch tomorrow"}`); status != http.StatusOK {
		t.Fatalf("status = %d, body %v", status, body)
	}

	offered := map[string]bool{}
	names := openai.lastTools()
	for _, n := range names {
		offered[n] = true
	}
	if !offered["list_events"] || offered["create_event"] {
		t.Errorf("tools = %v, want list_events offered and create_event not", names)
	}
}

// The Webhook Conversation integration sends every exposed entity and the
// conversation so far alongside the query.
func TestAgentConverseAnswersAFullHomeAssistantPayloadWithManyExposedEntities(t *testing.T) {
	openai, hits := fakeResponsesAPI(t, "It's raining.")
	app := agentApp(t, true, "sk-test", openai.URL)

	entities := make([]map[string]any, 2500)
	for i := range entities {
		entities[i] = map[string]any{
			"entity_id": fmt.Sprintf("light.room_%d_ceiling", i),
			"name":      fmt.Sprintf("Room %d ceiling light", i),
			"state":     "off",
			"aliases":   []string{fmt.Sprintf("room %d light", i)},
			"area_id":   fmt.Sprintf("room_%d", i),
			"area_name": fmt.Sprintf("Room %d", i),
		}
	}
	payload, _ := json.Marshal(map[string]any{
		"conversation_id":  "01J8",
		"user_id":          "abc",
		"user_name":        "Jordan",
		"language":         "en-CA",
		"agent_id":         "conversation.ziggy",
		"device_id":        "echo",
		"device_info":      map[string]any{"name": "Kitchen Echo", "manufacturer": "Amazon", "model": "Echo Dot"},
		"messages":         []map[string]any{{"role": "user", "content": "Is it raining?"}},
		"query":            "Is it raining?",
		"exposed_entities": entities,
		"system_prompt":    "",
		"stream":           false,
	})
	if len(payload) < 256<<10 {
		t.Fatalf("payload is %d bytes, want a large one", len(payload))
	}

	status, body := postConverse(t, app, string(payload))

	if status != http.StatusOK || body["output"] != "It's raining." {
		t.Errorf("status = %d, body = %v, want 200 with the reply", status, body)
	}
	if hits.Load() != 1 {
		t.Errorf("OpenAI hits = %d, want 1", hits.Load())
	}
}

func TestAgentConverseRejectsOversizedBody(t *testing.T) {
	openai, hits := fakeResponsesAPI(t, "unused")
	app := agentApp(t, true, "sk-test", openai.URL)

	body := `{"conversation_id":"c","query":"` + strings.Repeat("a", 5<<20) + `"}`
	if status, _ := postConverse(t, app, body); status != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", status)
	}
	if hits.Load() != 0 {
		t.Errorf("OpenAI hits = %d, want 0", hits.Load())
	}
}

func TestAgentConverseRejectsMissingOrBlankQuery(t *testing.T) {
	openai, hits := fakeResponsesAPI(t, "unused")
	app := agentApp(t, true, "sk-test", openai.URL)

	for _, body := range []string{`{"conversation_id":"c"}`, `{"conversation_id":"c","query":"   "}`, `not json`} {
		if status, _ := postConverse(t, app, body); status != http.StatusBadRequest {
			t.Errorf("POST %s: status = %d, want 400", body, status)
		}
	}
	if hits.Load() != 0 {
		t.Errorf("OpenAI hits = %d, want 0", hits.Load())
	}
}

func TestAgentConverseExplainsMissingAPIKeyAsSpokenReply(t *testing.T) {
	openai, _ := fakeResponsesAPI(t, "unused")
	app := agentApp(t, true, "", openai.URL)

	status, body := postConverse(t, app, `{"conversation_id":"c","query":"hello"}`)

	if status != http.StatusOK {
		t.Errorf("status = %d, want 200 so Home Assistant speaks the explanation", status)
	}
	if s, _ := body["output"].(string); strings.TrimSpace(s) == "" {
		t.Errorf("body = %v, want a spoken explanation", body)
	}
}

func TestWSSnapshotIncludesAgentState(t *testing.T) {
	openai, _ := fakeResponsesAPI(t, "unused")
	app := agentApp(t, true, "sk-test", openai.URL)
	conn, ctx := dialWS(t, app)

	snap := readFrame(t, ctx, conn)

	if snap.Type != "snapshot" {
		t.Fatalf("first frame type = %q, want snapshot", snap.Type)
	}
	if snap.Agent == nil {
		t.Fatalf("snapshot has no agent state")
	}
	if snap.Agent.State != types.AgentIdle || !snap.Agent.Ready || snap.Agent.CalendarWritable {
		t.Errorf("agent = %+v, want idle, ready, calendar not writable (no Google credentials)", snap.Agent)
	}
}

func TestAgentConverseBroadcastsThinkingThenTalking(t *testing.T) {
	openai, _ := fakeResponsesAPI(t, "Tomorrow you have the dentist.")
	app := agentApp(t, true, "sk-test", openai.URL)
	conn, ctx := dialWS(t, app)
	readFrame(t, ctx, conn) // initial snapshot

	if status, body := postConverse(t, app, `{"conversation_id":"c","query":"What's tomorrow?"}`); status != http.StatusOK {
		t.Fatalf("status = %d, body %v", status, body)
	}

	var agentFrames []types.AgentSnapshot
	for len(agentFrames) < 2 {
		f := readFrame(t, ctx, conn)
		if f.Type == "agent" && f.Agent != nil {
			agentFrames = append(agentFrames, *f.Agent)
		}
	}
	if agentFrames[0].State != types.AgentThinking || agentFrames[0].Heard != "What's tomorrow?" {
		t.Errorf("first agent frame = %+v, want thinking with heard text", agentFrames[0])
	}
	if agentFrames[1].State != types.AgentTalking || agentFrames[1].Reply != "Tomorrow you have the dentist." {
		t.Errorf("second agent frame = %+v, want talking with the reply", agentFrames[1])
	}
}

func getAgent(t *testing.T, app *httptest.Server) (int, types.AgentSnapshot) {
	t.Helper()
	resp, err := http.Get(app.URL + "/api/agent")
	if err != nil {
		t.Fatalf("GET /api/agent: %v", err)
	}
	defer resp.Body.Close()
	var snap types.AgentSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		t.Fatalf("decode /api/agent: %v", err)
	}
	return resp.StatusCode, snap
}

func TestGetAgentReportsReadyIdleAndReadOnlyCalendar(t *testing.T) {
	openai, _ := fakeResponsesAPI(t, "unused")
	app := agentApp(t, true, "sk-test", openai.URL)

	status, snap := getAgent(t, app)

	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if snap.State != types.AgentIdle || !snap.Ready || snap.CalendarWritable {
		t.Errorf("agent = %+v, want idle, ready, calendar not writable (no Google credentials)", snap)
	}
}

func TestGetAgentReportsNotReadyWithoutAPIKey(t *testing.T) {
	openai, _ := fakeResponsesAPI(t, "unused")
	app := agentApp(t, true, "", openai.URL)

	status, snap := getAgent(t, app)

	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if snap.Ready {
		t.Errorf("agent = %+v, want not ready without any credential", snap)
	}
}

// Home Assistant includes the voice satellite's device_id only when the
// question was spoken to a device, which will also speak the answer.
func TestAgentConverseMarksTurnsFromAVoiceDeviceAsSpoken(t *testing.T) {
	openai, _ := fakeResponsesAPI(t, "Hi there.")
	app := agentApp(t, true, "sk-test", openai.URL)

	cases := []struct {
		name, body string
		spoken     bool
	}{
		{"voice satellite", `{"conversation_id":"c","query":"hello","device_id":"echo"}`, true},
		{"typed", `{"conversation_id":"c","query":"hello"}`, false},
		{"blank device", `{"conversation_id":"c","query":"hello","device_id":""}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if status, body := postConverse(t, app, tc.body); status != http.StatusOK {
				t.Fatalf("converse status = %d, body %v", status, body)
			}
			if _, snap := getAgent(t, app); snap.Spoken != tc.spoken {
				t.Errorf("spoken = %v, want %v", snap.Spoken, tc.spoken)
			}
		})
	}
}

func TestGetAgentReflectsLatestConversationState(t *testing.T) {
	openai, _ := fakeResponsesAPI(t, "Hi there.")
	app := agentApp(t, true, "sk-test", openai.URL)
	if status, body := postConverse(t, app, `{"conversation_id":"c","query":"hello"}`); status != http.StatusOK {
		t.Fatalf("converse status = %d, body %v", status, body)
	}

	_, snap := getAgent(t, app)

	if snap.State != types.AgentTalking || snap.Heard != "hello" || snap.Reply != "Hi there." {
		t.Errorf("agent = %+v, want talking with the last exchange", snap)
	}
}

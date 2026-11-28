package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jpatters/home-calendar/internal/agent"
	"github.com/jpatters/home-calendar/internal/types"
)

const usageLimitReply = "I've hit the ChatGPT plan's usage limit for now."

// fakeOpenAI emulates POST /responses as a server-sent event stream. Each
// request gets the next scripted reply; once the script runs out the last
// reply repeats.
type fakeOpenAI struct {
	t      *testing.T
	srv    *httptest.Server
	mu     sync.Mutex
	reqs   []map[string]any
	auths  []string
	script []scripted
}

type scripted struct {
	// status and body answer with a plain JSON error instead of a stream.
	status int
	body   string
	hang   bool
	// response is the final response object, streamed as
	// response.created, text deltas, then response.completed.
	response string
	// events, when set, are streamed verbatim (each a JSON event) instead.
	events []string
}

func newFakeOpenAI(t *testing.T, script ...scripted) *fakeOpenAI {
	t.Helper()
	f := &fakeOpenAI{t: t, script: script}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

// toolNamespace is the name of the namespace the request groups its tools
// under, so scripted function calls can name it like a real model would.
func toolNamespace(body map[string]any) string {
	tools, _ := body["tools"].([]any)
	for _, raw := range tools {
		if tool, _ := raw.(map[string]any); tool["type"] == "namespace" {
			return fmt.Sprint(tool["name"])
		}
	}
	return ""
}

func (f *fakeOpenAI) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/responses") {
		http.NotFound(w, r)
		return
	}
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		f.t.Errorf("request is not JSON: %v: %s", err, raw)
	}
	f.mu.Lock()
	f.reqs = append(f.reqs, body)
	f.auths = append(f.auths, r.Header.Get("Authorization"))
	n := len(f.reqs)
	var s scripted
	if len(f.script) > 0 {
		s = f.script[min(n, len(f.script))-1]
	}
	f.mu.Unlock()
	if s.hang {
		<-r.Context().Done()
		return
	}
	if s.status != 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(s.status)
		io.WriteString(w, s.body)
		return
	}
	events := s.events
	if events == nil {
		events = streamOf(s.response)
	}
	w.Header().Set("Content-Type", "text/event-stream")
	ns := toolNamespace(body)
	for i, ev := range events {
		ev = strings.ReplaceAll(ev, "__NS__", ns)
		ev = strings.Replace(ev, `"sequence_number":0`, fmt.Sprintf(`"sequence_number":%d`, i), 1)
		var probe struct {
			Type string `json:"type"`
		}
		json.Unmarshal([]byte(ev), &probe)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", probe.Type, ev)
		if fl, ok := w.(http.Flusher); ok {
			fl.Flush()
		}
	}
}

// streamOf turns a final response object into the events a real stream
// carries: created, one delta per output text, completed.
func streamOf(response string) []string {
	var resp struct {
		ID     string `json:"id"`
		Output []struct {
			Type    string `json:"type"`
			ID      string `json:"id"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal([]byte(response), &resp); err != nil {
		panic(fmt.Sprintf("bad scripted response %s: %v", response, err))
	}
	created := fmt.Sprintf(`{"type":"response.created","sequence_number":0,"response":{"id":%q,"object":"response","created_at":1764200000,"status":"in_progress","model":"gpt-test","output":[]}}`, resp.ID)
	events := []string{created}
	for i, item := range resp.Output {
		if item.Type != "message" {
			continue
		}
		for _, c := range item.Content {
			events = append(events, fmt.Sprintf(`{"type":"response.output_text.delta","sequence_number":0,"item_id":%q,"output_index":%d,"content_index":0,"delta":%q,"logprobs":[]}`, item.ID, i, c.Text))
		}
	}
	return append(events, fmt.Sprintf(`{"type":"response.completed","sequence_number":0,"response":%s}`, response))
}

func (f *fakeOpenAI) requests() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]map[string]any(nil), f.reqs...)
}

func (f *fakeOpenAI) authorizations() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.auths...)
}

// text is a completed response with a reasoning item (carrying encrypted
// content) and a spoken reply.
func text(id, reply string) scripted {
	return scripted{response: fmt.Sprintf(`{"id":%q,"object":"response","created_at":1764200000,"status":"completed","model":"gpt-test","output":[{"type":"reasoning","id":"rs_%s","summary":[],"encrypted_content":"enc_%s"},{"type":"message","id":"msg_%s","role":"assistant","status":"completed","content":[{"type":"output_text","text":%q,"annotations":[]}]}]}`, id, id, id, id, reply)}
}

type fnCall struct{ callID, name, args string }

// calls is a completed response whose output is function calls in the
// request's tool namespace.
func calls(id string, cs ...fnCall) scripted {
	items := []string{fmt.Sprintf(`{"type":"reasoning","id":"rs_%s","summary":[],"encrypted_content":"enc_%s"}`, id, id)}
	for _, c := range cs {
		items = append(items, fmt.Sprintf(`{"type":"function_call","id":"fc_%s","call_id":%q,"namespace":"__NS__","name":%q,"arguments":%q,"status":"completed"}`, c.callID, c.callID, c.name, c.args))
	}
	return scripted{response: fmt.Sprintf(`{"id":%q,"object":"response","created_at":1764200000,"status":"completed","model":"gpt-test","output":[%s]}`, id, strings.Join(items, ","))}
}

// failed streams a response that starts and then fails with code.
func failed(id, code string) scripted {
	return scripted{events: []string{
		fmt.Sprintf(`{"type":"response.created","sequence_number":0,"response":{"id":%q,"object":"response","created_at":1764200000,"status":"in_progress","model":"gpt-test","output":[]}}`, id),
		fmt.Sprintf(`{"type":"response.output_text.delta","sequence_number":0,"item_id":"msg_%s","output_index":0,"content_index":0,"delta":"Let me","logprobs":[]}`, id),
		fmt.Sprintf(`{"type":"response.failed","sequence_number":0,"response":{"id":%q,"object":"response","created_at":1764200000,"status":"failed","model":"gpt-test","output":[],"error":{"code":%q,"message":"upstream detail"}}}`, id, code),
	}}
}

// recordingTool is a real tool that remembers the arguments it was called
// with and answers with a fixed result or error.
type recordingTool struct {
	name   string
	result any
	err    error
	mu     sync.Mutex
	args   []string
}

func (r *recordingTool) Name() string        { return r.name }
func (r *recordingTool) Description() string { return "Test tool " + r.name }
func (r *recordingTool) Schema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           map[string]any{"x": map[string]any{"type": "integer"}},
		"required":             []string{"x"},
		"additionalProperties": false,
	}
}
func (r *recordingTool) Call(ctx context.Context, args json.RawMessage) (any, error) {
	r.mu.Lock()
	r.args = append(r.args, string(args))
	r.mu.Unlock()
	return r.result, r.err
}
func (r *recordingTool) calls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.args...)
}

type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}
func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

func halifax(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/Halifax")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}
	return loc
}

func testConfig() types.Config {
	c := types.DefaultConfig()
	c.Agent.Enabled = true
	c.Agent.Model = "gpt-test"
	c.Agent.Effort = "medium"
	c.Calendars = []types.Calendar{
		{ID: "1", Name: "Family", URL: "https://calendar.google.com/calendar/ical/fam%40group.calendar.google.com/private-abc/basic.ics"},
		{ID: "2", Name: "Hockey Practice", URL: "https://example.com/hockey.ics"},
	}
	return c
}

func staticToken(tok string) func(context.Context) (string, error) {
	return func(context.Context) (string, error) { return tok, nil }
}

// newAgent builds a ready agent talking to fake with a fixed clock at
// 2026-11-26 22:30 in Halifax (02:30 UTC on the 27th).
func newAgent(t *testing.T, fake *fakeOpenAI, edit func(*agent.Options), tools ...agent.Tool) (*agent.Agent, *clock) {
	t.Helper()
	clk := &clock{now: time.Date(2026, 11, 27, 2, 30, 0, 0, time.UTC)}
	opts := agent.Options{
		Token:            staticToken("sk-test"),
		Ready:            true,
		BaseURL:          fake.srv.URL,
		Config:           testConfig,
		Tools:            agent.NewRegistry(tools...),
		CalendarWritable: true,
		Now:              clk.Now,
		Location:         halifax(t),
	}
	if edit != nil {
		edit(&opts)
	}
	return agent.New(opts), clk
}

func converse(t *testing.T, a *agent.Agent, conv, text string) string {
	t.Helper()
	reply, err := a.Converse(context.Background(), conv, text)
	if err != nil {
		t.Fatalf("Converse: %v", err)
	}
	return reply
}

func inputItems(req map[string]any) []map[string]any {
	var out []map[string]any
	items, _ := req["input"].([]any)
	for _, it := range items {
		m, _ := it.(map[string]any)
		out = append(out, m)
	}
	return out
}

func contentText(c any) string {
	switch v := c.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, p := range v {
			m, _ := p.(map[string]any)
			parts = append(parts, fmt.Sprint(m["text"]))
		}
		return strings.Join(parts, "")
	}
	return fmt.Sprint(c)
}

// transcript renders a request's input items as readable lines:
// "user: ...", "assistant: ...", "call <id> <namespace>.<name> <args>",
// "output <id> <output>", or "<type>" for anything else.
func transcript(req map[string]any) []string {
	var out []string
	for _, m := range inputItems(req) {
		switch {
		case m["type"] == "function_call":
			out = append(out, fmt.Sprintf("call %v %v.%v %v", m["call_id"], m["namespace"], m["name"], m["arguments"]))
		case m["type"] == "function_call_output":
			out = append(out, fmt.Sprintf("output %v %v", m["call_id"], contentText(m["output"])))
		case m["role"] != nil && (m["type"] == nil || m["type"] == "message"):
			out = append(out, fmt.Sprintf("%v: %s", m["role"], contentText(m["content"])))
		default:
			out = append(out, fmt.Sprint(m["type"]))
		}
	}
	return out
}

// functionOutputs returns call_id -> output for every function_call_output
// item in a request's input.
func functionOutputs(t *testing.T, req map[string]any) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, m := range inputItems(req) {
		if m["type"] == "function_call_output" {
			out[fmt.Sprint(m["call_id"])] = contentText(m["output"])
		}
	}
	return out
}

// offeredTools returns every function tool in a request, including those
// grouped in namespaces.
func offeredTools(req map[string]any) []map[string]any {
	var out []map[string]any
	tools, _ := req["tools"].([]any)
	for _, raw := range tools {
		tool, _ := raw.(map[string]any)
		if tool["type"] == "namespace" {
			inner, _ := tool["tools"].([]any)
			for _, r := range inner {
				m, _ := r.(map[string]any)
				out = append(out, m)
			}
			continue
		}
		out = append(out, tool)
	}
	return out
}

func toolNames(req map[string]any) []string {
	var names []string
	for _, tool := range offeredTools(req) {
		names = append(names, fmt.Sprint(tool["name"]))
	}
	sort.Strings(names)
	return names
}

func asJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// request returns the i-th request the fake received, failing the test when
// there were not that many.
func request(t *testing.T, f *fakeOpenAI, i int) map[string]any {
	t.Helper()
	reqs := f.requests()
	if len(reqs) <= i {
		t.Fatalf("model received %d requests, want at least %d", len(reqs), i+1)
	}
	return reqs[i]
}

func wantTranscript(t *testing.T, req map[string]any, want ...string) {
	t.Helper()
	if got := transcript(req); !reflect.DeepEqual(got, want) {
		t.Errorf("input =\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

func TestConverseReturnsStreamedModelTextAnswer(t *testing.T) {
	fake := newFakeOpenAI(t, text("resp_1", "It is sunny today."))
	a, _ := newAgent(t, fake, nil)

	reply := converse(t, a, "conv-1", "What's the weather?")

	if reply != "It is sunny today." {
		t.Errorf("reply = %q, want the model's text", reply)
	}
	if n := len(fake.requests()); n != 1 {
		t.Fatalf("requests = %d, want 1", n)
	}
	wantTranscript(t, request(t, fake, 0), "user: What's the weather?")
}

// Sign in with ChatGPT accepts only store:false + stream:true requests
// without previous_response_id or system-role items, so every request uses
// that shape whichever credential is in use.
func TestEveryRequestIsStreamedUnstoredAndSelfContained(t *testing.T) {
	fake := newFakeOpenAI(t,
		calls("resp_1", fnCall{"call_1", "lookup", `{"x":1}`}),
		text("resp_2", "Here."),
		text("resp_3", "Again."),
	)
	a, _ := newAgent(t, fake, nil, &recordingTool{name: "lookup", result: "x"})

	converse(t, a, "conv-1", "first")
	converse(t, a, "conv-1", "second")

	reqs := fake.requests()
	if len(reqs) != 3 {
		t.Fatalf("requests = %d, want 3", len(reqs))
	}
	for i, req := range reqs {
		if req["store"] != false {
			t.Errorf("request %d store = %v, want false", i, req["store"])
		}
		if req["stream"] != true {
			t.Errorf("request %d stream = %v, want true", i, req["stream"])
		}
		if v, ok := req["previous_response_id"]; ok {
			t.Errorf("request %d sent previous_response_id %v", i, v)
		}
		if s, _ := req["instructions"].(string); !strings.Contains(s, "Ziggy") {
			t.Errorf("request %d instructions = %q, want Ziggy's instructions", i, s)
		}
		for _, item := range inputItems(req) {
			if item["role"] == "system" {
				t.Errorf("request %d has a system-role input item %v", i, item)
			}
		}
	}
}

func TestConverseOffersRegisteredToolsAsStrictFunctionsInANamespace(t *testing.T) {
	fake := newFakeOpenAI(t, text("resp_1", "ok"))
	a, _ := newAgent(t, fake, nil, &recordingTool{name: "alpha"}, &recordingTool{name: "beta"})

	converse(t, a, "conv-1", "hello")

	req := request(t, fake, 0)
	tools, _ := req["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("tools = %s, want one namespace grouping the functions", asJSON(tools))
	}
	ns, _ := tools[0].(map[string]any)
	if ns["type"] != "namespace" || strings.TrimSpace(fmt.Sprint(ns["name"])) == "" || strings.TrimSpace(fmt.Sprint(ns["description"])) == "" {
		t.Errorf("tool group = %s, want a named, described namespace", asJSON(ns))
	}
	for _, tool := range offeredTools(req) {
		if tool["type"] != "function" || tool["strict"] != true {
			t.Errorf("tool %v is not a strict function", tool)
		}
		params, _ := tool["parameters"].(map[string]any)
		if params["additionalProperties"] != false {
			t.Errorf("tool %v parameters lost the schema", tool["name"])
		}
	}
	if got := toolNames(req); !reflect.DeepEqual(got, []string{"alpha", "beta"}) {
		t.Errorf("tools = %v, want alpha and beta", got)
	}
}

func TestConverseRunsRequestedToolAndResendsTheTurnWithItsOutput(t *testing.T) {
	tool := &recordingTool{name: "lookup", result: map[string]any{"answer": "forty-two"}}
	fake := newFakeOpenAI(t,
		calls("resp_1", fnCall{"call_1", "lookup", `{"x":7}`}),
		text("resp_2", "The answer is forty-two."),
	)
	a, _ := newAgent(t, fake, nil, tool)

	reply := converse(t, a, "conv-1", "What's the answer?")

	if reply != "The answer is forty-two." {
		t.Errorf("reply = %q, want the final text", reply)
	}
	if got := tool.calls(); len(got) != 1 || got[0] != `{"x":7}` {
		t.Errorf("tool calls = %v, want one call with {\"x\":7}", got)
	}
	followUp := request(t, fake, 1)
	ns := toolNamespace(followUp)
	wantTranscript(t, followUp,
		"user: What's the answer?",
		fmt.Sprintf(`call call_1 %s.lookup {"x":7}`, ns),
		`output call_1 {"answer":"forty-two"}`,
	)
}

func TestConverseRunsEveryToolCallInOneResponse(t *testing.T) {
	first := &recordingTool{name: "first", result: "one"}
	second := &recordingTool{name: "second", result: "two"}
	fake := newFakeOpenAI(t,
		calls("resp_1", fnCall{"call_a", "first", `{"x":1}`}, fnCall{"call_b", "second", `{"x":2}`}),
		text("resp_2", "Done."),
	)
	a, _ := newAgent(t, fake, nil, first, second)

	converse(t, a, "conv-1", "do both")

	if len(first.calls()) != 1 || len(second.calls()) != 1 {
		t.Errorf("calls: first=%v second=%v, want one each", first.calls(), second.calls())
	}
	out := functionOutputs(t, request(t, fake, 1))
	if !strings.Contains(out["call_a"], "one") || !strings.Contains(out["call_b"], "two") {
		t.Errorf("function outputs = %v, want both results", out)
	}
}

func TestConverseReportsToolErrorToModelAndContinues(t *testing.T) {
	tool := &recordingTool{name: "flaky", err: errors.New("the hot tub did not answer")}
	fake := newFakeOpenAI(t,
		calls("resp_1", fnCall{"call_1", "flaky", `{"x":1}`}),
		text("resp_2", "Sorry, the hot tub isn't responding."),
	)
	a, _ := newAgent(t, fake, nil, tool)

	reply := converse(t, a, "conv-1", "hot tub?")

	if reply != "Sorry, the hot tub isn't responding." {
		t.Errorf("reply = %q, want the model's follow-up", reply)
	}
	out := functionOutputs(t, request(t, fake, 1))
	if !strings.Contains(out["call_1"], "the hot tub did not answer") {
		t.Errorf("function outputs = %v, want the tool's error message", out)
	}
}

func TestConverseReportsUnknownToolToModelAndContinues(t *testing.T) {
	fake := newFakeOpenAI(t,
		calls("resp_1", fnCall{"call_1", "teleport", `{}`}),
		text("resp_2", "I can't do that."),
	)
	a, _ := newAgent(t, fake, nil, &recordingTool{name: "lookup"})

	reply := converse(t, a, "conv-1", "teleport me")

	if reply != "I can't do that." {
		t.Errorf("reply = %q, want the model's follow-up", reply)
	}
	out := functionOutputs(t, request(t, fake, 1))
	if !strings.Contains(out["call_1"], "teleport") {
		t.Errorf("function outputs = %v, want an error naming the unknown tool", out)
	}
}

func TestConverseGivesUpAfterSixRounds(t *testing.T) {
	tool := &recordingTool{name: "loop", result: "again"}
	fake := newFakeOpenAI(t, calls("resp_n", fnCall{"call_1", "loop", `{"x":1}`}))
	a, _ := newAgent(t, fake, nil, tool)

	reply, err := a.Converse(context.Background(), "conv-1", "spin")

	if n := len(fake.requests()); n != 6 {
		t.Errorf("model requests = %d, want 6", n)
	}
	if err == nil {
		t.Errorf("err = nil, want an error reporting the abandoned turn")
	}
	if strings.TrimSpace(reply) == "" {
		t.Errorf("reply is empty, want a spoken fallback")
	}
}

// A failed turn may end in function calls that never got outputs, so the
// whole conversation is dropped and the next turn starts from nothing.
func TestConverseStartsFreshAfterAFailedTurn(t *testing.T) {
	loop := calls("resp_loop", fnCall{"call_1", "loop", `{"x":1}`})
	cases := []struct {
		name   string
		script []scripted
		// index of the first request after the failed turn
		next    int
		timeout time.Duration
	}{
		{"round cap", []scripted{text("resp_ok", "Hi."), loop, loop, loop, loop, loop, loop, text("resp_next", "Hi.")}, 7, 0},
		{"timeout mid-loop", []scripted{text("resp_ok", "Hi."), loop, {hang: true}, text("resp_next", "Hi.")}, 3, 200 * time.Millisecond},
		{"model error mid-loop", []scripted{text("resp_ok", "Hi."), loop, {status: http.StatusBadRequest, body: `{"error":{"message":"nope","type":"invalid_request_error"}}`}, text("resp_next", "Hi.")}, 3, 0},
		{"stream failure mid-loop", []scripted{text("resp_ok", "Hi."), loop, failed("resp_bad", "server_error"), text("resp_next", "Hi.")}, 3, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeOpenAI(t, tc.script...)
			a, _ := newAgent(t, fake, func(o *agent.Options) {
				if tc.timeout > 0 {
					o.Timeout = tc.timeout
				}
			}, &recordingTool{name: "loop", result: "again"})

			converse(t, a, "conv-1", "hello")
			if _, err := a.Converse(context.Background(), "conv-1", "spin"); err == nil {
				t.Errorf("failed turn err = nil, want the failure reported")
			}
			converse(t, a, "conv-1", "hello again")

			wantTranscript(t, request(t, fake, tc.next), "user: hello again")
		})
	}
}

func TestConverseResendsConversationWithinFiveMinutes(t *testing.T) {
	fake := newFakeOpenAI(t, text("resp_1", "Sure."), text("resp_2", "Done."))
	a, clk := newAgent(t, fake, nil, &recordingTool{name: "lookup"})

	converse(t, a, "conv-1", "Add soccer on Saturday.")
	clk.Advance(4 * time.Minute)
	converse(t, a, "conv-1", "At ten.")

	wantTranscript(t, request(t, fake, 1),
		"user: Add soccer on Saturday.",
		"assistant: Sure.",
		"user: At ten.",
	)
}

func TestFollowUpResendsEarlierToolCallsAndOutputs(t *testing.T) {
	tool := &recordingTool{name: "lookup", result: "x"}
	fake := newFakeOpenAI(t,
		calls("resp_1", fnCall{"call_1", "lookup", `{"x":1}`}),
		text("resp_2", "Here."),
		text("resp_3", "Again."),
	)
	a, _ := newAgent(t, fake, nil, tool)

	converse(t, a, "conv-1", "first")
	converse(t, a, "conv-1", "second")

	third := request(t, fake, 2)
	wantTranscript(t, third,
		"user: first",
		fmt.Sprintf(`call call_1 %s.lookup {"x":1}`, toolNamespace(third)),
		`output call_1 "x"`,
		"assistant: Here.",
		"user: second",
	)
}

// Nothing is stored server-side, so resent history must not refer to stored
// items: reasoning items and output item ids are dropped.
func TestResentHistoryCarriesNoServerSideReferences(t *testing.T) {
	fake := newFakeOpenAI(t,
		calls("resp_1", fnCall{"call_1", "lookup", `{"x":1}`}),
		text("resp_2", "Here."),
		text("resp_3", "Again."),
	)
	a, _ := newAgent(t, fake, nil, &recordingTool{name: "lookup", result: "x"})

	converse(t, a, "conv-1", "first")
	converse(t, a, "conv-1", "second")

	for i, req := range fake.requests() {
		for _, item := range inputItems(req) {
			if item["type"] == "reasoning" || item["type"] == "item_reference" {
				t.Errorf("request %d resends %v item %s", i, item["type"], asJSON(item))
			}
			if _, ok := item["id"]; ok {
				t.Errorf("request %d resends item id in %s", i, asJSON(item))
			}
		}
	}
}

func TestConverseConversationTimeoutSlidesFromTheLastTurn(t *testing.T) {
	fake := newFakeOpenAI(t, text("resp_1", "Sure."), text("resp_2", "Okay."), text("resp_3", "Done."))
	a, clk := newAgent(t, fake, nil)

	converse(t, a, "conv-1", "Add soccer.")
	clk.Advance(4 * time.Minute)
	converse(t, a, "conv-1", "On Saturday.")
	clk.Advance(4 * time.Minute)
	converse(t, a, "conv-1", "At ten.")

	wantTranscript(t, request(t, fake, 2),
		"user: Add soccer.",
		"assistant: Sure.",
		"user: On Saturday.",
		"assistant: Okay.",
		"user: At ten.",
	)
}

func TestConverseForgetsConversationAfterFiveMinutes(t *testing.T) {
	fake := newFakeOpenAI(t, text("resp_1", "Sure."), text("resp_2", "Hi."))
	a, clk := newAgent(t, fake, nil)

	converse(t, a, "conv-1", "Add soccer on Saturday.")
	clk.Advance(5*time.Minute + time.Second)
	converse(t, a, "conv-1", "Hello.")

	wantTranscript(t, request(t, fake, 1), "user: Hello.")
}

func TestConverseKeepsConversationsSeparate(t *testing.T) {
	fake := newFakeOpenAI(t, text("resp_1", "Sure."), text("resp_2", "Hi."))
	a, _ := newAgent(t, fake, nil)

	converse(t, a, "kitchen", "Add soccer on Saturday.")
	converse(t, a, "bedroom", "Hello.")

	wantTranscript(t, request(t, fake, 1), "user: Hello.")
}

func TestConverseRereadsModelAndEffortEveryTurn(t *testing.T) {
	var mu sync.Mutex
	cfg := testConfig()
	fake := newFakeOpenAI(t, text("resp_1", "ok"), text("resp_2", "ok"))
	a, _ := newAgent(t, fake, func(o *agent.Options) {
		o.Config = func() types.Config {
			mu.Lock()
			defer mu.Unlock()
			return cfg
		}
	})

	converse(t, a, "conv-1", "hi")
	first := request(t, fake, 0)
	if reasoning, _ := first["reasoning"].(map[string]any); first["model"] != "gpt-test" || reasoning["effort"] != "medium" {
		t.Errorf("first turn model=%v reasoning=%v, want gpt-test / medium", first["model"], first["reasoning"])
	}
	mu.Lock()
	cfg.Agent.Model = "gpt-other"
	cfg.Agent.Effort = "high"
	mu.Unlock()
	converse(t, a, "conv-1", "again")

	second := request(t, fake, 1)
	reasoning, _ := second["reasoning"].(map[string]any)
	if second["model"] != "gpt-other" || reasoning["effort"] != "high" {
		t.Errorf("second turn model=%v reasoning=%v, want gpt-other / high", second["model"], second["reasoning"])
	}
}

func TestConverseSendsTheCurrentCredentialAsBearerOnEveryRequest(t *testing.T) {
	var mu sync.Mutex
	token := "chatgpt-access-1"
	fake := newFakeOpenAI(t,
		calls("resp_1", fnCall{"call_1", "lookup", `{"x":1}`}),
		text("resp_2", "Here."),
		text("resp_3", "Again."),
	)
	a, _ := newAgent(t, fake, func(o *agent.Options) {
		o.Token = func(context.Context) (string, error) {
			mu.Lock()
			defer mu.Unlock()
			return token, nil
		}
	}, &recordingTool{name: "lookup", result: "x"})

	converse(t, a, "conv-1", "first")
	mu.Lock()
	token = "sk-api-key"
	mu.Unlock()
	converse(t, a, "conv-1", "second")

	want := []string{"Bearer chatgpt-access-1", "Bearer chatgpt-access-1", "Bearer sk-api-key"}
	if got := fake.authorizations(); !reflect.DeepEqual(got, want) {
		t.Errorf("Authorization headers = %v, want %v", got, want)
	}
}

func TestConverseAsksToSignInAgainWhenChatGPTSignInIsLost(t *testing.T) {
	var mu sync.Mutex
	var last types.AgentSnapshot
	fake := newFakeOpenAI(t, text("resp_1", "unused"))
	a, _ := newAgent(t, fake, func(o *agent.Options) {
		o.Token = func(context.Context) (string, error) {
			return "", fmt.Errorf("refresh: %w", agent.ErrSignInAgain)
		}
		o.OnState = func(s *types.AgentSnapshot) {
			mu.Lock()
			last = *s
			mu.Unlock()
		}
	})

	reply, err := a.Converse(context.Background(), "conv-1", "hi")

	if err == nil {
		t.Errorf("err = nil, want the lost sign-in reported")
	}
	lower := strings.ToLower(reply)
	if !strings.Contains(lower, "sign in again") || !strings.Contains(lower, "calendar settings") {
		t.Errorf("reply = %q, want it to ask for signing in again in the calendar settings", reply)
	}
	if n := len(fake.requests()); n != 0 {
		t.Errorf("model requests = %d, want 0", n)
	}
	mu.Lock()
	defer mu.Unlock()
	if last.State != types.AgentError || last.Reply != reply {
		t.Errorf("last state = %+v, want error with the spoken reply", last)
	}
}

func TestConverseApologisesWhenCredentialLookupFails(t *testing.T) {
	fake := newFakeOpenAI(t, text("resp_1", "unused"))
	a, _ := newAgent(t, fake, func(o *agent.Options) {
		o.Token = func(context.Context) (string, error) { return "", errors.New("auth server unreachable") }
	})

	reply, err := a.Converse(context.Background(), "conv-1", "hi")

	if err == nil || strings.TrimSpace(reply) == "" {
		t.Errorf("reply %q err %v, want a spoken apology and the error", reply, err)
	}
	if strings.Contains(reply, "unreachable") {
		t.Errorf("reply %q leaks the internal error", reply)
	}
	if n := len(fake.requests()); n != 0 {
		t.Errorf("model requests = %d, want 0", n)
	}
}

func TestConverseSpeaksUsageLimitWhenStreamFailsPartway(t *testing.T) {
	fake := newFakeOpenAI(t, failed("resp_1", "subscription_sharing_usage_limit_exceeded"), text("resp_2", "Hi."))
	a, _ := newAgent(t, fake, nil)

	reply, err := a.Converse(context.Background(), "conv-1", "hi")

	if reply != usageLimitReply {
		t.Errorf("reply = %q, want %q", reply, usageLimitReply)
	}
	if err == nil {
		t.Errorf("err = nil, want the failed response reported")
	}
	if snap := a.Snapshot(); snap.State != types.AgentError || snap.Reply != usageLimitReply {
		t.Errorf("Snapshot() = %+v, want error with the usage-limit reply", snap)
	}
	converse(t, a, "conv-1", "hello again")
	wantTranscript(t, request(t, fake, 1), "user: hello again")
}

func TestConverseSpeaksUsageLimitWhenRejectedBeforeStreaming(t *testing.T) {
	fake := newFakeOpenAI(t, scripted{status: http.StatusTooManyRequests, body: `{"error":{"code":"subscription_sharing_usage_limit_exceeded","message":"usage limit reached","type":"rate_limit_error","param":null}}`})
	a, _ := newAgent(t, fake, nil)

	reply, err := a.Converse(context.Background(), "conv-1", "hi")

	if reply != usageLimitReply {
		t.Errorf("reply = %q, want %q", reply, usageLimitReply)
	}
	if err == nil {
		t.Errorf("err = nil, want the rejection reported")
	}
}

func TestConverseTreatsOtherStreamFailuresAsErrors(t *testing.T) {
	fake := newFakeOpenAI(t, failed("resp_1", "server_error"))
	a, _ := newAgent(t, fake, nil)

	reply, err := a.Converse(context.Background(), "conv-1", "hi")

	if err == nil || strings.TrimSpace(reply) == "" || reply == usageLimitReply {
		t.Errorf("reply %q err %v, want a generic spoken apology and the error", reply, err)
	}
	if strings.Contains(reply, "upstream detail") || strings.Contains(reply, "Let me") {
		t.Errorf("reply %q leaks the failed stream", reply)
	}
}

func TestConverseTreatsStreamEndingWithoutCompletionAsFailure(t *testing.T) {
	fake := newFakeOpenAI(t, scripted{events: []string{
		`{"type":"response.created","sequence_number":0,"response":{"id":"resp_1","object":"response","created_at":1764200000,"status":"in_progress","model":"gpt-test","output":[]}}`,
		`{"type":"response.output_text.delta","sequence_number":0,"item_id":"msg_1","output_index":0,"content_index":0,"delta":"Half an ans","logprobs":[]}`,
	}})
	a, _ := newAgent(t, fake, nil)

	reply, err := a.Converse(context.Background(), "conv-1", "hi")

	if err == nil {
		t.Errorf("err = nil, want a stream without response.completed treated as a failure")
	}
	if strings.TrimSpace(reply) == "" || strings.Contains(reply, "Half an ans") {
		t.Errorf("reply = %q, want a spoken apology, not the partial text", reply)
	}
}

func TestInstructionsIncludeLocalDateWeekdayAndZone(t *testing.T) {
	fake := newFakeOpenAI(t, text("resp_1", "ok"))
	a, _ := newAgent(t, fake, nil)

	converse(t, a, "conv-1", "what day is it")

	instructions, _ := request(t, fake, 0)["instructions"].(string)
	for _, want := range []string{"Ziggy", "2026-11-26", "Thursday", "22:30", "America/Halifax"} {
		if !strings.Contains(instructions, want) {
			t.Errorf("instructions missing %q:\n%s", want, instructions)
		}
	}
	if strings.Contains(instructions, "2026-11-27") {
		t.Errorf("instructions use the UTC date, want local:\n%s", instructions)
	}
}

func instructionsAt(t *testing.T, at time.Time, cfg func(*types.Config)) string {
	t.Helper()
	fake := newFakeOpenAI(t, text("resp_1", "ok"))
	a, clk := newAgent(t, fake, func(o *agent.Options) {
		c := testConfig()
		if cfg != nil {
			cfg(&c)
		}
		o.Config = func() types.Config { return c }
	})
	if !at.IsZero() {
		clk.mu.Lock()
		clk.now = at
		clk.mu.Unlock()
	}
	converse(t, a, "conv-1", "hi")
	instructions, _ := request(t, fake, 0)["instructions"].(string)
	return instructions
}

func TestInstructionsSayWhatZiggyIsWearing(t *testing.T) {
	christmasAfternoon := time.Date(2026, 12, 25, 15, 0, 0, 0, halifax(t))

	instructions := instructionsAt(t, christmasAfternoon, nil)

	if !strings.Contains(instructions, agent.DescribeOutfit("christmas")) {
		t.Errorf("instructions don't describe the Christmas outfit:\n%s", instructions)
	}
	if strings.Contains(instructions, agent.DescribeOutfit("pajamas")) {
		t.Errorf("instructions put Ziggy in pajamas in the afternoon:\n%s", instructions)
	}
}

func TestInstructionsAtNightSayZiggyGotOutOfBedInPajamas(t *testing.T) {
	night := time.Date(2026, 11, 27, 2, 30, 0, 0, halifax(t))

	instructions := instructionsAt(t, night, nil)

	for _, want := range []string{agent.DescribeOutfit("pajamas"), "out of bed"} {
		if !strings.Contains(instructions, want) {
			t.Errorf("instructions missing %q:\n%s", want, instructions)
		}
	}
}

func TestInstructionsInTheDaytimeDontMentionGettingOutOfBed(t *testing.T) {
	instructions := instructionsAt(t, time.Date(2026, 11, 27, 14, 0, 0, 0, halifax(t)), nil)

	if strings.Contains(instructions, "out of bed") {
		t.Errorf("daytime instructions mention getting out of bed:\n%s", instructions)
	}
}

func TestInstructionsNameTheTeamsZiggyFollows(t *testing.T) {
	instructions := instructionsAt(t, time.Time{}, func(c *types.Config) {
		c.Baseball = types.Baseball{Enabled: true, TeamID: 141, TeamName: "Toronto Blue Jays", TeamAbbr: "TOR"}
		c.Hockey = types.Hockey{Enabled: true, TeamAbbr: "TOR", TeamName: "Toronto Maple Leafs"}
	})

	for _, want := range []string{"Toronto Blue Jays", "Toronto Maple Leafs"} {
		if !strings.Contains(instructions, want) {
			t.Errorf("instructions missing team %q:\n%s", want, instructions)
		}
	}
}

func TestInstructionsLeaveOutTeamsThatArentFollowed(t *testing.T) {
	instructions := instructionsAt(t, time.Time{}, func(c *types.Config) {
		c.Baseball = types.Baseball{Enabled: false, TeamID: 141, TeamName: "Toronto Blue Jays"}
		c.Hockey = types.Hockey{Enabled: true}
	})

	if strings.Contains(instructions, "Blue Jays") || strings.Contains(instructions, "fan of") {
		t.Errorf("instructions name a team that isn't followed:\n%s", instructions)
	}
}

func TestInstructionsIntroduceBiscuit(t *testing.T) {
	if instructions := instructionsAt(t, time.Time{}, nil); !strings.Contains(instructions, "Biscuit") {
		t.Errorf("instructions don't mention Biscuit:\n%s", instructions)
	}
}

func TestInstructionsNameConfiguredCalendars(t *testing.T) {
	fake := newFakeOpenAI(t, text("resp_1", "ok"))
	a, _ := newAgent(t, fake, nil)

	converse(t, a, "conv-1", "hi")

	instructions, _ := request(t, fake, 0)["instructions"].(string)
	for _, want := range []string{"Family", "Hockey Practice"} {
		if !strings.Contains(instructions, want) {
			t.Errorf("instructions missing calendar %q:\n%s", want, instructions)
		}
	}
}

func TestConverseReportsThinkingThenTalking(t *testing.T) {
	var mu sync.Mutex
	var states []types.AgentSnapshot
	fake := newFakeOpenAI(t, text("resp_1", "It is sunny."))
	a, _ := newAgent(t, fake, func(o *agent.Options) {
		o.OnState = func(s *types.AgentSnapshot) {
			mu.Lock()
			states = append(states, *s)
			mu.Unlock()
		}
	})

	converse(t, a, "conv-1", "Weather?")

	mu.Lock()
	defer mu.Unlock()
	if len(states) < 2 {
		t.Fatalf("states = %+v, want thinking then talking", states)
	}
	first, last := states[0], states[len(states)-1]
	if first.State != types.AgentThinking || first.Heard != "Weather?" || first.Reply != "" {
		t.Errorf("first state = %+v, want thinking with heard text and no reply", first)
	}
	if last.State != types.AgentTalking || last.Reply != "It is sunny." || last.Heard != "Weather?" {
		t.Errorf("last state = %+v, want talking with the reply", last)
	}
	if !last.Ready || !last.CalendarWritable {
		t.Errorf("last state = %+v, want ready and calendar writable", last)
	}
	if got := a.Snapshot(); got == nil || got.State != types.AgentTalking || got.Reply != "It is sunny." {
		t.Errorf("Snapshot() = %+v, want the talking state", got)
	}
}

func TestConverseSpokenMarksEveryStateOfItsTurn(t *testing.T) {
	var mu sync.Mutex
	var states []types.AgentSnapshot
	weather := &recordingTool{name: "get_weather", result: "rain"}
	fake := newFakeOpenAI(t,
		calls("resp_1", fnCall{"call_1", "get_weather", `{}`}),
		text("resp_2", "Rainy."),
		text("resp_3", "Hello."),
	)
	a, _ := newAgent(t, fake, func(o *agent.Options) {
		o.OnState = func(s *types.AgentSnapshot) {
			mu.Lock()
			states = append(states, *s)
			mu.Unlock()
		}
	}, weather)

	if _, err := a.ConverseSpoken(context.Background(), "conv-1", "Weather?"); err != nil {
		t.Fatalf("ConverseSpoken: %v", err)
	}
	mu.Lock()
	spokenTurn := append([]types.AgentSnapshot(nil), states...)
	states = nil
	mu.Unlock()
	converse(t, a, "conv-2", "Hi")

	mu.Lock()
	defer mu.Unlock()
	for _, s := range spokenTurn {
		if !s.Spoken {
			t.Errorf("spoken turn state %s (tool %q) is not marked spoken", s.State, s.Tool)
		}
	}
	for _, s := range states {
		if s.Spoken {
			t.Errorf("typed turn state %s is marked spoken", s.State)
		}
	}
}

func TestConverseReportsEachToolAsItRunsThenClearsIt(t *testing.T) {
	var mu sync.Mutex
	var states []types.AgentSnapshot
	weather := &recordingTool{name: "get_weather", result: "rain"}
	scores := &recordingTool{name: "get_scores", result: "3-1"}
	fake := newFakeOpenAI(t,
		calls("resp_1", fnCall{"call_1", "get_weather", `{}`}),
		calls("resp_2", fnCall{"call_2", "get_scores", `{}`}),
		text("resp_3", "Rainy, and the Leafs won."),
	)
	a, _ := newAgent(t, fake, func(o *agent.Options) {
		o.OnState = func(s *types.AgentSnapshot) {
			mu.Lock()
			states = append(states, *s)
			mu.Unlock()
		}
	}, weather, scores)

	converse(t, a, "conv-1", "Weather and score?")

	mu.Lock()
	defer mu.Unlock()
	var got []string
	for _, s := range states {
		got = append(got, fmt.Sprintf("%s:%s", s.State, s.Tool))
	}
	want := []string{"thinking:", "thinking:get_weather", "thinking:get_scores", "talking:"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("states = %v, want %v", got, want)
	}
	for _, s := range states {
		if s.Heard != "Weather and score?" {
			t.Errorf("state %+v lost the heard text", s)
		}
	}
}

func TestConcurrentTurnsOnOneConversationEachGetAReplyAndPublishInOrder(t *testing.T) {
	var mu sync.Mutex
	var states []types.AgentSnapshot
	talking := 0
	secondTalking := make(chan struct{})
	fake := newFakeOpenAI(t, text("resp_1", "ok"))
	a, _ := newAgent(t, fake, func(o *agent.Options) {
		// The first turn to finish is slow to publish its reply: it waits
		// (briefly) for the other turn's reply to be published first.
		o.OnState = func(s *types.AgentSnapshot) {
			mu.Lock()
			if s.State == types.AgentTalking {
				talking++
				if talking == 1 {
					mu.Unlock()
					select {
					case <-secondTalking:
					case <-time.After(500 * time.Millisecond):
					}
					mu.Lock()
				} else {
					close(secondTalking)
				}
			}
			states = append(states, *s)
			mu.Unlock()
		}
	})

	var wg sync.WaitGroup
	replies := make([]string, 2)
	for i := range replies {
		wg.Add(1)
		go func() {
			defer wg.Done()
			replies[i], _ = a.Converse(context.Background(), "conv-1", fmt.Sprintf("question %d", i))
		}()
	}
	wg.Wait()

	for i, r := range replies {
		if r != "ok" {
			t.Errorf("turn %d reply = %q, want the model's answer", i, r)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if last, snap := states[len(states)-1], a.Snapshot(); last != *snap {
		t.Errorf("last published state = %+v, but Snapshot() = %+v", last, *snap)
	}
}

func TestConverseTimesOutWithSpokenError(t *testing.T) {
	var mu sync.Mutex
	var last types.AgentSnapshot
	fake := newFakeOpenAI(t, scripted{hang: true})
	a, _ := newAgent(t, fake, func(o *agent.Options) {
		o.Timeout = 200 * time.Millisecond
		o.OnState = func(s *types.AgentSnapshot) {
			mu.Lock()
			last = *s
			mu.Unlock()
		}
	})

	start := time.Now()
	reply, err := a.Converse(context.Background(), "conv-1", "slow question")
	elapsed := time.Since(start)

	if err == nil {
		t.Errorf("err = nil, want a timeout error")
	}
	if strings.TrimSpace(reply) == "" {
		t.Errorf("reply is empty, want a spoken apology")
	}
	if elapsed > 5*time.Second {
		t.Errorf("Converse took %v, want it to honour the 200ms timeout", elapsed)
	}
	mu.Lock()
	defer mu.Unlock()
	if last.State != types.AgentError {
		t.Errorf("last state = %+v, want error", last)
	}
}

func TestConverseReportsModelFailureAsErrorState(t *testing.T) {
	var mu sync.Mutex
	var last types.AgentSnapshot
	fake := newFakeOpenAI(t, scripted{status: http.StatusBadRequest, body: `{"error":{"message":"bad model","type":"invalid_request_error"}}`})
	a, _ := newAgent(t, fake, func(o *agent.Options) {
		o.OnState = func(s *types.AgentSnapshot) {
			mu.Lock()
			last = *s
			mu.Unlock()
		}
	})

	reply, err := a.Converse(context.Background(), "conv-1", "hi")

	if err == nil {
		t.Errorf("err = nil, want the API failure")
	}
	if strings.TrimSpace(reply) == "" {
		t.Errorf("reply is empty, want a spoken apology")
	}
	if strings.Contains(reply, "bad model") {
		t.Errorf("reply %q leaks the API error", reply)
	}
	mu.Lock()
	defer mu.Unlock()
	if last.State != types.AgentError {
		t.Errorf("last state = %+v, want error", last)
	}
}

func TestConverseWithoutCredentialsExplainsWithoutCallingModel(t *testing.T) {
	cases := map[string]func(*agent.Options){
		"not ready":   func(o *agent.Options) { o.Ready = false },
		"empty token": func(o *agent.Options) { o.Token = staticToken("") },
	}
	for name, edit := range cases {
		t.Run(name, func(t *testing.T) {
			fake := newFakeOpenAI(t, text("resp_1", "should not be used"))
			a, _ := newAgent(t, fake, edit)

			reply, err := a.Converse(context.Background(), "conv-1", "hi")

			if strings.TrimSpace(reply) == "" || err == nil {
				t.Errorf("reply %q err %v, want a spoken explanation and an error", reply, err)
			}
			if n := len(fake.requests()); n != 0 {
				t.Errorf("model requests = %d, want 0", n)
			}
		})
	}
}

func TestSnapshotIsIdleAndReportsCapabilitiesBeforeAnyTurn(t *testing.T) {
	fake := newFakeOpenAI(t)
	a, _ := newAgent(t, fake, func(o *agent.Options) { o.CalendarWritable = false })

	snap := a.Snapshot()

	if snap == nil {
		t.Fatalf("Snapshot() = nil, want idle state")
	}
	if snap.State != types.AgentIdle || !snap.Ready || snap.CalendarWritable {
		t.Errorf("Snapshot() = %+v, want idle, ready, not calendar writable", snap)
	}
}

func TestSetCapabilitiesTakesEffectOnTheNextTurnAndIsPublished(t *testing.T) {
	var mu sync.Mutex
	var published []types.AgentSnapshot
	fake := newFakeOpenAI(t, text("resp_1", "ok"))
	a, _ := newAgent(t, fake, func(o *agent.Options) {
		o.Ready = false
		o.CalendarWritable = false
		o.OnState = func(s *types.AgentSnapshot) {
			mu.Lock()
			published = append(published, *s)
			mu.Unlock()
		}
	}, &recordingTool{name: "alpha"})

	a.SetCapabilities(agent.Capabilities{
		Ready:            true,
		CalendarWritable: true,
		Tools:            agent.NewRegistry(&recordingTool{name: "beta"}),
	})

	if snap := a.Snapshot(); snap.State != types.AgentIdle || !snap.Ready || !snap.CalendarWritable {
		t.Errorf("Snapshot() = %+v, want idle, ready and calendar writable", snap)
	}
	mu.Lock()
	if len(published) != 1 || !published[0].Ready || !published[0].CalendarWritable || published[0].State != types.AgentIdle {
		t.Errorf("published = %+v, want one idle frame with the new capabilities", published)
	}
	mu.Unlock()

	converse(t, a, "conv-1", "hi")

	req := request(t, fake, 0)
	if got := toolNames(req); !reflect.DeepEqual(got, []string{"beta"}) {
		t.Errorf("tools = %v, want only the new registry's beta", got)
	}
	if s, _ := req["instructions"].(string); !strings.Contains(s, "add, change and delete") {
		t.Errorf("instructions do not say calendar changes are possible:\n%s", s)
	}

	a.SetCapabilities(agent.Capabilities{Ready: false, Tools: agent.NewRegistry()})
	if _, err := a.Converse(context.Background(), "conv-1", "again"); err == nil {
		t.Errorf("Converse after losing credentials err = nil, want not ready")
	}
	if n := len(fake.requests()); n != 1 {
		t.Errorf("model requests = %d, want no request once not ready", n)
	}
}

func TestSetCapabilitiesIsSafeDuringTurns(t *testing.T) {
	fake := newFakeOpenAI(t,
		calls("resp_1", fnCall{"call_1", "lookup", `{"x":1}`}),
		text("resp_2", "ok"),
	)
	a, _ := newAgent(t, fake, nil, &recordingTool{name: "lookup", result: "x"})

	var wg sync.WaitGroup
	replies := make([]string, 10)
	errs := make([]error, 10)
	for i := range replies {
		wg.Add(2)
		go func() {
			defer wg.Done()
			replies[i], errs[i] = a.Converse(context.Background(), fmt.Sprintf("conv-%d", i), "hi")
		}()
		go func() {
			defer wg.Done()
			a.SetCapabilities(agent.Capabilities{Ready: true, CalendarWritable: i%2 == 0, Tools: agent.NewRegistry(&recordingTool{name: "lookup", result: "x"})})
		}()
	}
	wg.Wait()

	for i := range replies {
		if errs[i] != nil || replies[i] != "ok" {
			t.Errorf("turn %d = %q, %v; want the model's answer while capabilities change", i, replies[i], errs[i])
		}
	}
	a.SetCapabilities(agent.Capabilities{Ready: true, CalendarWritable: true, Tools: agent.NewRegistry()})
	if snap := a.Snapshot(); !snap.Ready || !snap.CalendarWritable {
		t.Errorf("Snapshot() = %+v, want the last capabilities set", snap)
	}
}

func TestConversationKeepsOnlyTheTenMostRecentTurns(t *testing.T) {
	script := []scripted{calls("resp_tool", fnCall{"call_1", "lookup", `{"x":1}`})}
	for i := 1; i <= 13; i++ {
		script = append(script, text(fmt.Sprintf("resp_%d", i), fmt.Sprintf("answer %d", i)))
	}
	fake := newFakeOpenAI(t, script...)
	a, clk := newAgent(t, fake, nil, &recordingTool{name: "lookup", result: "x"})

	// Turn 1 uses a tool (two requests); turns 2-12 are plain.
	for i := 1; i <= 12; i++ {
		converse(t, a, "conv-1", fmt.Sprintf("turn %d", i))
		clk.Advance(time.Minute)
	}
	converse(t, a, "conv-1", "turn 13")

	last := transcript(request(t, fake, 13))
	var users []string
	for _, line := range last {
		if strings.HasPrefix(line, "call ") || strings.HasPrefix(line, "output ") {
			t.Errorf("dropped turn's tool item still sent: %s", line)
		}
		if strings.HasPrefix(line, "user: ") {
			users = append(users, strings.TrimPrefix(line, "user: "))
		}
	}
	want := []string{"turn 3", "turn 4", "turn 5", "turn 6", "turn 7", "turn 8", "turn 9", "turn 10", "turn 11", "turn 12", "turn 13"}
	if !reflect.DeepEqual(users, want) {
		t.Errorf("user turns sent = %v, want the ten most recent plus the new one %v", users, want)
	}
	if len(last) == 0 || last[0] != "user: turn 3" {
		t.Errorf("history starts with %v, want it to start at turn 3", last)
	}
}

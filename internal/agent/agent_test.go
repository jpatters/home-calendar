package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jpatters/home-calendar/internal/agent"
	"github.com/jpatters/home-calendar/internal/types"
)

// fakeOpenAI emulates POST /responses. Each request gets the next scripted
// reply; once the script runs out the last reply repeats.
type fakeOpenAI struct {
	t      *testing.T
	srv    *httptest.Server
	mu     sync.Mutex
	reqs   []map[string]any
	script []scripted
}

type scripted struct {
	status int
	body   string
	hang   bool
}

func newFakeOpenAI(t *testing.T, script ...scripted) *fakeOpenAI {
	t.Helper()
	f := &fakeOpenAI{t: t, script: script}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
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
	w.Header().Set("Content-Type", "application/json")
	if s.status != 0 {
		w.WriteHeader(s.status)
	}
	io.WriteString(w, s.body)
}

func (f *fakeOpenAI) requests() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]map[string]any(nil), f.reqs...)
}

func text(id, reply string) scripted {
	return scripted{body: fmt.Sprintf(`{"id":%q,"object":"response","created_at":1764200000,"status":"completed","model":"gpt-test","output":[{"type":"reasoning","id":"rs_%s","summary":[]},{"type":"message","id":"msg_%s","role":"assistant","status":"completed","content":[{"type":"output_text","text":%q,"annotations":[]}]}]}`, id, id, id, reply)}
}

type fnCall struct{ callID, name, args string }

func calls(id string, cs ...fnCall) scripted {
	items := make([]string, len(cs))
	for i, c := range cs {
		items[i] = fmt.Sprintf(`{"type":"function_call","id":"fc_%s","call_id":%q,"name":%q,"arguments":%q,"status":"completed"}`, c.callID, c.callID, c.name, c.args)
	}
	return scripted{body: fmt.Sprintf(`{"id":%q,"object":"response","created_at":1764200000,"status":"completed","model":"gpt-test","output":[%s]}`, id, strings.Join(items, ","))}
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

// newAgent builds an agent talking to fake with a fixed clock at
// 2026-11-26 22:30 in Halifax (02:30 UTC on the 27th).
func newAgent(t *testing.T, fake *fakeOpenAI, edit func(*agent.Options), tools ...agent.Tool) (*agent.Agent, *clock) {
	t.Helper()
	clk := &clock{now: time.Date(2026, 11, 27, 2, 30, 0, 0, time.UTC)}
	opts := agent.Options{
		APIKey:           "sk-test",
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

// functionOutputs returns call_id -> output for every function_call_output
// item in a request's input.
func functionOutputs(t *testing.T, req map[string]any) map[string]string {
	t.Helper()
	out := map[string]string{}
	items, _ := req["input"].([]any)
	for _, it := range items {
		m, _ := it.(map[string]any)
		if m["type"] != "function_call_output" {
			continue
		}
		id, _ := m["call_id"].(string)
		switch o := m["output"].(type) {
		case string:
			out[id] = o
		default:
			b, _ := json.Marshal(o)
			out[id] = string(b)
		}
	}
	return out
}

func asJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestConverseReturnsModelTextAnswer(t *testing.T) {
	fake := newFakeOpenAI(t, text("resp_1", "It is sunny today."))
	a, _ := newAgent(t, fake, nil)

	reply := converse(t, a, "conv-1", "What's the weather?")

	if reply != "It is sunny today." {
		t.Errorf("reply = %q, want the model's text", reply)
	}
	reqs := fake.requests()
	if len(reqs) != 1 {
		t.Fatalf("requests = %d, want 1", len(reqs))
	}
	if !strings.Contains(asJSON(request(t, fake, 0)["input"]), "What's the weather?") {
		t.Errorf("input %s does not carry the user's words", asJSON(request(t, fake, 0)["input"]))
	}
}

func TestConverseSendsConfiguredModelAndEffort(t *testing.T) {
	fake := newFakeOpenAI(t, text("resp_1", "ok"))
	a, _ := newAgent(t, fake, nil)

	converse(t, a, "conv-1", "hello")

	req := request(t, fake, 0)
	if req["model"] != "gpt-test" {
		t.Errorf("model = %v, want gpt-test", req["model"])
	}
	reasoning, _ := req["reasoning"].(map[string]any)
	if reasoning["effort"] != "medium" {
		t.Errorf("reasoning = %v, want effort medium", req["reasoning"])
	}
}

func TestConverseOffersRegisteredToolsAsStrictFunctions(t *testing.T) {
	fake := newFakeOpenAI(t, text("resp_1", "ok"))
	a, _ := newAgent(t, fake, nil, &recordingTool{name: "alpha"}, &recordingTool{name: "beta"})

	converse(t, a, "conv-1", "hello")

	tools, _ := request(t, fake, 0)["tools"].([]any)
	var names []string
	for _, raw := range tools {
		tool, _ := raw.(map[string]any)
		if tool["type"] != "function" || tool["strict"] != true {
			t.Errorf("tool %v is not a strict function", tool)
		}
		params, _ := tool["parameters"].(map[string]any)
		if params["additionalProperties"] != false {
			t.Errorf("tool %v parameters lost the schema", tool["name"])
		}
		names = append(names, fmt.Sprint(tool["name"]))
	}
	if strings.Join(names, ",") != "alpha,beta" {
		t.Errorf("tools = %v, want [alpha beta] in registry order", names)
	}
}

func TestConverseRunsRequestedToolAndSendsOutputBack(t *testing.T) {
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
	reqs := fake.requests()
	if len(reqs) != 2 {
		t.Fatalf("requests = %d, want 2", len(reqs))
	}
	if request(t, fake, 1)["previous_response_id"] != "resp_1" {
		t.Errorf("previous_response_id = %v, want resp_1", request(t, fake, 1)["previous_response_id"])
	}
	out := functionOutputs(t, request(t, fake, 1))
	if !strings.Contains(out["call_1"], "forty-two") {
		t.Errorf("function outputs = %v, want call_1 carrying the tool result", out)
	}
	followUp := request(t, fake, 1)
	assertInstructionsAndTools(t, followUp, "lookup")
	for _, typ := range inputTypes(followUp) {
		if typ != "function_call_output" {
			t.Errorf("follow-up input item types = %v, want only function_call_output", inputTypes(followUp))
			break
		}
	}
	if strings.Contains(asJSON(followUp["input"]), "What's the answer?") {
		t.Errorf("follow-up input %s resends the user's words", asJSON(followUp["input"]))
	}
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

// A turn that ends with function calls the model never saw answered must not
// be chained to (the API rejects a previous_response_id with pending calls),
// and the conversation it belonged to is dropped rather than resumed from an
// earlier turn that no longer matches what was said.
func TestConverseStartsFreshAfterAnAbandonedTurn(t *testing.T) {
	loop := calls("resp_loop", fnCall{"call_1", "loop", `{"x":1}`})
	cases := []struct {
		name   string
		script []scripted
		// index of the first request after the abandoned turn
		next    int
		timeout time.Duration
	}{
		{"round cap", []scripted{text("resp_ok", "Hi."), loop, loop, loop, loop, loop, loop, text("resp_next", "Hi.")}, 7, 0},
		{"timeout mid-loop", []scripted{text("resp_ok", "Hi."), loop, {hang: true}, text("resp_next", "Hi.")}, 3, 200 * time.Millisecond},
		{"model error mid-loop", []scripted{text("resp_ok", "Hi."), loop, {status: http.StatusBadRequest, body: `{"error":{"message":"nope","type":"invalid_request_error"}}`}, text("resp_next", "Hi.")}, 3, 0},
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
				t.Errorf("first turn err = nil, want the failure reported")
			}
			converse(t, a, "conv-1", "hello again")

			next := request(t, fake, tc.next)
			if got, ok := next["previous_response_id"]; ok {
				t.Errorf("next turn chained to %v, want a fresh conversation", got)
			}
		})
	}
}

func TestConverseContinuesConversationWithinFiveMinutes(t *testing.T) {
	fake := newFakeOpenAI(t, text("resp_1", "Sure."), text("resp_2", "Done."))
	a, clk := newAgent(t, fake, nil, &recordingTool{name: "lookup"})

	converse(t, a, "conv-1", "Add soccer on Saturday.")
	clk.Advance(4 * time.Minute)
	converse(t, a, "conv-1", "At ten.")

	if _, ok := request(t, fake, 0)["previous_response_id"]; ok {
		t.Errorf("first turn sent previous_response_id %v", request(t, fake, 0)["previous_response_id"])
	}
	second := request(t, fake, 1)
	if second["previous_response_id"] != "resp_1" {
		t.Errorf("second turn previous_response_id = %v, want resp_1", second["previous_response_id"])
	}
	assertInstructionsAndTools(t, second, "lookup")
	if !strings.Contains(asJSON(second["input"]), "At ten.") {
		t.Errorf("second turn input %s does not carry the new words", asJSON(second["input"]))
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

	if got := request(t, fake, 2)["previous_response_id"]; got != "resp_2" {
		t.Errorf("third turn previous_response_id = %v, want resp_2 (8 minutes in, 4 since the last turn)", got)
	}
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

func TestConverseContinuesFromLastResponseOfPreviousTurn(t *testing.T) {
	tool := &recordingTool{name: "lookup", result: "x"}
	fake := newFakeOpenAI(t,
		calls("resp_1", fnCall{"call_1", "lookup", `{"x":1}`}),
		text("resp_2", "Here."),
		text("resp_3", "Again."),
	)
	a, _ := newAgent(t, fake, nil, tool)

	converse(t, a, "conv-1", "first")
	converse(t, a, "conv-1", "second")

	if got := request(t, fake, 2)["previous_response_id"]; got != "resp_2" {
		t.Errorf("previous_response_id = %v, want resp_2", got)
	}
}

func TestConverseForgetsConversationAfterFiveMinutes(t *testing.T) {
	fake := newFakeOpenAI(t, text("resp_1", "Sure."), text("resp_2", "Hi."))
	a, clk := newAgent(t, fake, nil)

	converse(t, a, "conv-1", "Add soccer on Saturday.")
	clk.Advance(5*time.Minute + time.Second)
	converse(t, a, "conv-1", "Hello.")

	if got, ok := request(t, fake, 1)["previous_response_id"]; ok {
		t.Errorf("expired conversation still sent previous_response_id %v", got)
	}
}

func TestConverseKeepsConversationsSeparate(t *testing.T) {
	fake := newFakeOpenAI(t, text("resp_1", "Sure."), text("resp_2", "Hi."))
	a, _ := newAgent(t, fake, nil)

	converse(t, a, "kitchen", "Add soccer on Saturday.")
	converse(t, a, "bedroom", "Hello.")

	if got, ok := request(t, fake, 1)["previous_response_id"]; ok {
		t.Errorf("other conversation sent previous_response_id %v", got)
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

// assertInstructionsAndTools checks req carries Ziggy's instructions and
// offers the named tools.
func assertInstructionsAndTools(t *testing.T, req map[string]any, toolNames ...string) {
	t.Helper()
	if s, _ := req["instructions"].(string); !strings.Contains(s, "Ziggy") {
		t.Errorf("request instructions = %q, want Ziggy's instructions", s)
	}
	offered := map[string]bool{}
	tools, _ := req["tools"].([]any)
	for _, raw := range tools {
		tool, _ := raw.(map[string]any)
		offered[fmt.Sprint(tool["name"])] = true
	}
	for _, n := range toolNames {
		if !offered[n] {
			t.Errorf("request tools = %v, want %q offered", req["tools"], n)
		}
	}
}

func inputTypes(req map[string]any) []string {
	var out []string
	items, _ := req["input"].([]any)
	for _, it := range items {
		m, _ := it.(map[string]any)
		out = append(out, fmt.Sprint(m["type"]))
	}
	return out
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

func TestConverseWithoutAPIKeyExplainsWithoutCallingModel(t *testing.T) {
	fake := newFakeOpenAI(t, text("resp_1", "should not be used"))
	a, _ := newAgent(t, fake, func(o *agent.Options) { o.APIKey = "" })

	reply, _ := a.Converse(context.Background(), "conv-1", "hi")

	if strings.TrimSpace(reply) == "" {
		t.Errorf("reply is empty, want a spoken explanation")
	}
	if n := len(fake.requests()); n != 0 {
		t.Errorf("model requests = %d, want 0", n)
	}
	if snap := a.Snapshot(); snap == nil || snap.Ready {
		t.Errorf("Snapshot() = %+v, want not ready", snap)
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

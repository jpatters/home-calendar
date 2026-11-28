package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jpatters/home-calendar/internal/homeassistant/hatest"
)

func fakeHome(t *testing.T) *hatest.Server {
	t.Helper()
	return hatest.New(t,
		[]hatest.Entity{{EntityID: "light.kitchen", State: "on", Exposed: true, AreaID: "kitchen",
			Attributes: map[string]any{"friendly_name": "Kitchen Lights"}}},
		[]hatest.Area{{ID: "kitchen", Name: "Kitchen"}}, nil)
}

type homeStatus struct {
	HomeAssistant struct {
		Set     bool   `json:"set"`
		FromEnv bool   `json:"fromEnv"`
		URL     string `json:"url"`
	} `json:"homeAssistant"`
	Error string `json:"error"`
}

func putHome(t *testing.T, base, body string) (int, homeStatus, string) {
	t.Helper()
	status, raw := memoriesRequest(t, http.MethodPut, base+"/api/agent/auth/home-assistant", body)
	var st homeStatus
	json.Unmarshal(raw, &st)
	return status, st, string(raw)
}

func getHomeStatus(t *testing.T, base string) homeStatus {
	t.Helper()
	_, raw := memoriesRequest(t, http.MethodGet, base+"/api/agent/auth", "")
	var st homeStatus
	if err := json.Unmarshal(raw, &st); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return st
}

func homeBody(url, token string) string {
	b, _ := json.Marshal(map[string]string{"url": url, "token": token})
	return string(b)
}

func TestConnectingHomeAssistantChecksTheTokenAndNeverEchoesIt(t *testing.T) {
	t.Setenv("HOME_ASSISTANT_TOKEN", "")
	openai, _ := fakeResponsesAPI(t, "ok")
	app := agentApp(t, true, "sk-test", openai.URL)
	ha := fakeHome(t)

	status, st, raw := putHome(t, app.URL, homeBody(ha.URL+"/", hatest.Token))

	if status != http.StatusOK || !st.HomeAssistant.Set || st.HomeAssistant.URL != ha.URL {
		t.Fatalf("PUT = %d %s, want connected with the URL", status, raw)
	}
	_, got := memoriesRequest(t, http.MethodGet, app.URL+"/api/agent/auth", "")
	for _, body := range []string{raw, string(got)} {
		if strings.Contains(body, hatest.Token) {
			t.Errorf("a response contains the token: %s", body)
		}
	}
	if st := getHomeStatus(t, app.URL); !st.HomeAssistant.Set || st.HomeAssistant.URL != ha.URL {
		t.Errorf("status = %+v, want Home Assistant connected", st.HomeAssistant)
	}

	if status, _ := memoriesRequest(t, http.MethodDelete, app.URL+"/api/agent/auth/home-assistant", ""); status != http.StatusOK {
		t.Errorf("DELETE = %d", status)
	}
	if st := getHomeStatus(t, app.URL); st.HomeAssistant.Set {
		t.Errorf("after DELETE = %+v, want disconnected", st.HomeAssistant)
	}
}

func TestADashboardAddressIsSavedAsTheServerAddress(t *testing.T) {
	t.Setenv("HOME_ASSISTANT_TOKEN", "")
	openai, _ := fakeResponsesAPI(t, "ok")
	app := agentApp(t, true, "sk-test", openai.URL)
	ha := fakeHome(t)

	status, st, raw := putHome(t, app.URL, homeBody(ha.URL+"/lovelace/0", hatest.Token))

	if status != http.StatusOK || st.HomeAssistant.URL != ha.URL {
		t.Errorf("PUT = %d %s, want it saved as %s", status, raw, ha.URL)
	}
}

func TestConnectingHomeAssistantRejectsABadTokenOrAddress(t *testing.T) {
	t.Setenv("HOME_ASSISTANT_TOKEN", "")
	openai, _ := fakeResponsesAPI(t, "ok")
	app := agentApp(t, true, "sk-test", openai.URL)
	ha := fakeHome(t)

	cases := []struct{ name, body, says string }{
		{"wrong token", homeBody(ha.URL, "nope"), "token was rejected"},
		{"not a web address", homeBody("homeassistant.local", hatest.Token), "address"},
		{"no token", homeBody(ha.URL, " "), "token"},
		{"nothing there", homeBody("http://127.0.0.1:1", hatest.Token), "reach Home Assistant"},
		{"a password in the address", homeBody(strings.Replace(ha.URL, "http://", "http://me:secret@", 1), hatest.Token), "address"},
		{"not Home Assistant", homeBody(openai.URL, hatest.Token), "isn't Home Assistant"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, st, raw := putHome(t, app.URL, tc.body)
			if status != http.StatusBadRequest || !strings.Contains(st.Error, tc.says) {
				t.Errorf("PUT = %d %s, want 400 saying %q", status, raw, tc.says)
			}
		})
	}
	if st := getHomeStatus(t, app.URL); st.HomeAssistant.Set {
		t.Errorf("a rejected connection was saved: %+v", st.HomeAssistant)
	}
}

func TestHomeAssistantSetByTheEnvironmentCantBeChangedInAdmin(t *testing.T) {
	ha := fakeHome(t)
	t.Setenv("HOME_ASSISTANT_URL", ha.URL)
	t.Setenv("HOME_ASSISTANT_TOKEN", hatest.Token)
	openai, _ := fakeResponsesAPI(t, "ok")
	app := agentApp(t, true, "sk-test", openai.URL)

	if status, _, raw := putHome(t, app.URL, homeBody(ha.URL, hatest.Token)); status != http.StatusConflict {
		t.Errorf("PUT = %d %s, want 409", status, raw)
	}
	if status, raw := memoriesRequest(t, http.MethodDelete, app.URL+"/api/agent/auth/home-assistant", ""); status != http.StatusConflict {
		t.Errorf("DELETE = %d %s, want 409", status, raw)
	}
	if st := getHomeStatus(t, app.URL).HomeAssistant; !st.Set || !st.FromEnv || st.URL != ha.URL {
		t.Errorf("status = %+v, want set by the environment with its URL", st)
	}
}

func TestZiggyCanReadTheHomeOnceHomeAssistantIsConnected(t *testing.T) {
	t.Setenv("HOME_ASSISTANT_TOKEN", "")
	openai := newFakeResponsesCalling(t, "list_devices", "{}", "The kitchen lights are on.")
	app := agentApp(t, true, "sk-test", openai.URL)
	ha := fakeHome(t)
	if status, _, raw := putHome(t, app.URL, homeBody(ha.URL, hatest.Token)); status != http.StatusOK {
		t.Fatalf("PUT = %d %s", status, raw)
	}

	postConverse(t, app, `{"conversation_id":"c","query":"are the kitchen lights on?"}`)

	bodies := openai.requestBodies()
	if len(bodies) < 2 || !strings.Contains(bodies[1], "Kitchen Lights") {
		t.Errorf("the tool result sent back to the model doesn't describe the kitchen lights: %v", bodies)
	}
}

func TestZiggyLosesTheHomeOnceHomeAssistantIsDisconnected(t *testing.T) {
	t.Setenv("HOME_ASSISTANT_TOKEN", "")
	openai := newFakeResponsesCalling(t, "list_devices", "{}", "I can't reach the house.")
	app := agentApp(t, true, "sk-test", openai.URL)
	ha := fakeHome(t)
	putHome(t, app.URL, homeBody(ha.URL, hatest.Token))
	memoriesRequest(t, http.MethodDelete, app.URL+"/api/agent/auth/home-assistant", "")

	postConverse(t, app, `{"conversation_id":"c","query":"are the kitchen lights on?"}`)

	bodies := openai.requestBodies()
	if len(bodies) < 2 || !strings.Contains(bodies[1], "Home Assistant isn't connected") {
		t.Errorf("after disconnecting, the tool result should say Home Assistant isn't connected: %v", bodies)
	}
}

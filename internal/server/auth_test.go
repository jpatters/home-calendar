package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/coder/websocket"

	"github.com/jpatters/home-calendar/internal/chatgpt/chatgpttest"
	"github.com/jpatters/home-calendar/internal/types"
)

type appEnv struct {
	apiKey string
	google string
	// expiresIn, when set, is the ChatGPT access-token lifetime in seconds.
	expiresIn int
}

// authTestApp is the full router with a fake OpenAI and a fake Sign in with
// ChatGPT server at the network edge.
type authTestApp struct {
	app    *httptest.Server
	url    string
	dir    string
	openai *fakeResponses
	auth   *chatgpttest.Server
	ws     *websocket.Conn
	wsCtx  context.Context
}

func newAuthTestApp(t *testing.T, env appEnv) *authTestApp {
	t.Helper()
	t.Setenv("OPENAI_API_KEY", env.apiKey)
	t.Setenv("GOOGLE_SERVICE_ACCOUNT_JSON", env.google)
	a := &authTestApp{
		dir:    t.TempDir(),
		openai: newFakeResponses(t, "Okay."),
		auth:   chatgpttest.New(t),
	}
	if env.expiresIn != 0 {
		a.auth.Lock()
		a.auth.ExpiresIn = env.expiresIn
		a.auth.Unlock()
	}
	t.Setenv("OPENAI_BASE_URL", a.openai.URL)
	a.restart(t)
	return a
}

// restart starts a new server on the same data directory.
func (a *authTestApp) restart(t *testing.T) {
	t.Helper()
	app := startApp(t, a.dir, true, a.auth.URL)
	a.app = app
	a.url = app.URL
	a.ws, a.wsCtx = dialWS(t, app)
	readFrame(t, a.wsCtx, a.ws) // initial snapshot
}

func (a *authTestApp) do(t *testing.T, method, path, body string) (int, []byte) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, a.url+path, r)
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

func decodeStatus(t *testing.T, raw []byte) AuthStatus {
	t.Helper()
	var st AuthStatus
	if err := json.Unmarshal(raw, &st); err != nil {
		t.Fatalf("decode auth status %s: %v", raw, err)
	}
	return st
}

func errorMessage(t *testing.T, raw []byte) string {
	t.Helper()
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &body); err != nil || strings.TrimSpace(body.Error) == "" {
		t.Errorf("body %s, want {\"error\": message}", raw)
	}
	return body.Error
}

func (a *authTestApp) status(t *testing.T) AuthStatus {
	t.Helper()
	code, raw := a.do(t, http.MethodGet, "/api/agent/auth", "")
	if code != http.StatusOK {
		t.Fatalf("GET /api/agent/auth = %d %s", code, raw)
	}
	return decodeStatus(t, raw)
}

func (a *authTestApp) agent(t *testing.T) types.AgentSnapshot {
	t.Helper()
	code, raw := a.do(t, http.MethodGet, "/api/agent", "")
	var snap types.AgentSnapshot
	if code != http.StatusOK || json.Unmarshal(raw, &snap) != nil {
		t.Fatalf("GET /api/agent = %d %s", code, raw)
	}
	return snap
}

func (a *authTestApp) startSignIn(t *testing.T) string {
	t.Helper()
	code, raw := a.do(t, http.MethodPost, "/api/agent/auth/chatgpt/start", "")
	var body struct {
		AuthorizeURL string `json:"authorizeUrl"`
	}
	if code != http.StatusOK || json.Unmarshal(raw, &body) != nil || body.AuthorizeURL == "" {
		t.Fatalf("start = %d %s, want 200 with authorizeUrl", code, raw)
	}
	return body.AuthorizeURL
}

func completeBody(redirect string) string {
	b, _ := json.Marshal(map[string]string{"redirectUrl": redirect})
	return string(b)
}

func (a *authTestApp) signIn(t *testing.T) AuthStatus {
	t.Helper()
	authURL := a.startSignIn(t)
	code, raw := a.do(t, http.MethodPost, "/api/agent/auth/chatgpt/complete", completeBody(a.auth.Callback(authURL)))
	if code != http.StatusOK {
		t.Fatalf("complete = %d %s, want 200", code, raw)
	}
	return decodeStatus(t, raw)
}

func (a *authTestApp) converse(t *testing.T, query string) string {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"conversation_id": "c", "query": query})
	code, raw := a.do(t, http.MethodPost, "/api/agent/converse", string(b))
	var body struct {
		Output string `json:"output"`
	}
	if code != http.StatusOK || json.Unmarshal(raw, &body) != nil {
		t.Fatalf("converse = %d %s", code, raw)
	}
	return body.Output
}

// nextAgentFrame reads frames until an agent frame satisfying ok arrives.
func (a *authTestApp) nextAgentFrame(t *testing.T, ok func(types.AgentSnapshot) bool) types.AgentSnapshot {
	t.Helper()
	for {
		f := readFrame(t, a.wsCtx, a.ws)
		if f.Type == "agent" && f.Agent != nil && ok(*f.Agent) {
			return *f.Agent
		}
	}
}

func serviceAccountJSON(t *testing.T, email string) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	b, _ := json.Marshal(map[string]string{
		"type":           "service_account",
		"project_id":     "test-project",
		"private_key_id": "abc123",
		"private_key":    string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
		"client_email":   email,
		"client_id":      "1234567890",
		"token_uri":      "https://oauth2.googleapis.com/token",
	})
	return string(b)
}

func googleBody(json_ string) string {
	b, _ := json.Marshal(map[string]string{"json": json_})
	return string(b)
}

func TestAgentAuthStatusWithNothingConfigured(t *testing.T) {
	a := newAuthTestApp(t, appEnv{})

	got := a.status(t)

	want := AuthStatus{Using: "none"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("status = %+v, want %+v", got, want)
	}
	if a.agent(t).Ready {
		t.Errorf("agent ready with no credentials")
	}
}

func TestAPIKeyCanBeSetAndClearedFromAdmin(t *testing.T) {
	a := newAuthTestApp(t, appEnv{})

	code, raw := a.do(t, http.MethodPut, "/api/agent/auth/api-key", `{"key":"sk-admin"}`)

	if code != http.StatusOK {
		t.Fatalf("PUT api-key = %d %s, want 200", code, raw)
	}
	if st := decodeStatus(t, raw); st.Using != "apiKey" || st.APIKey != (SecretStatus{Set: true}) {
		t.Errorf("status = %+v, want using a stored api key", st)
	}
	if !a.agent(t).Ready {
		t.Errorf("agent not ready after setting an api key")
	}
	a.restart(t)
	if st := a.status(t); st.Using != "apiKey" || !st.APIKey.Set {
		t.Errorf("status after restart = %+v, want the stored key kept", st)
	}

	code, raw = a.do(t, http.MethodDelete, "/api/agent/auth/api-key", "")

	if code != http.StatusOK {
		t.Fatalf("DELETE api-key = %d %s, want 200", code, raw)
	}
	if st := decodeStatus(t, raw); st.Using != "none" || st.APIKey.Set {
		t.Errorf("status = %+v, want the key gone", st)
	}
	if a.agent(t).Ready {
		t.Errorf("agent still ready after removing the api key")
	}
}

func TestAPIKeyRejectsBlankOrMalformedBodies(t *testing.T) {
	a := newAuthTestApp(t, appEnv{})

	for _, body := range []string{`{"key":"   "}`, `{}`, `not json`} {
		code, raw := a.do(t, http.MethodPut, "/api/agent/auth/api-key", body)
		if code != http.StatusBadRequest {
			t.Errorf("PUT %s = %d, want 400", body, code)
		}
		errorMessage(t, raw)
	}
	if a.status(t).APIKey.Set {
		t.Errorf("a rejected body stored a key")
	}
}

func TestEnvironmentCredentialsCannotBeChangedFromAdmin(t *testing.T) {
	sa := serviceAccountJSON(t, "env@test-project.iam.gserviceaccount.com")
	a := newAuthTestApp(t, appEnv{apiKey: "sk-env", google: sa})

	st := a.status(t)
	if st.Using != "apiKey" || st.APIKey != (SecretStatus{Set: true, FromEnv: true}) {
		t.Errorf("api key status = %+v, want set from env", st)
	}
	if st.Google != (GoogleStatus{Set: true, FromEnv: true, ServiceAccountEmail: "env@test-project.iam.gserviceaccount.com"}) {
		t.Errorf("google status = %+v, want set from env with its email", st.Google)
	}
	for _, req := range []struct{ method, path, body string }{
		{http.MethodPut, "/api/agent/auth/api-key", `{"key":"sk-admin"}`},
		{http.MethodDelete, "/api/agent/auth/api-key", ""},
		{http.MethodPut, "/api/agent/auth/google", googleBody(serviceAccountJSON(t, "admin@test-project.iam.gserviceaccount.com"))},
		{http.MethodDelete, "/api/agent/auth/google", ""},
	} {
		code, raw := a.do(t, req.method, req.path, req.body)
		if code != http.StatusConflict {
			t.Errorf("%s %s = %d, want 409", req.method, req.path, code)
		}
		errorMessage(t, raw)
	}
	a.converse(t, "hello")
	if got := a.openai.authorizations(); len(got) != 1 || got[0] != "Bearer sk-env" {
		t.Errorf("Authorization = %v, want the env key still in use", got)
	}
	if a.status(t).Google.ServiceAccountEmail != "env@test-project.iam.gserviceaccount.com" {
		t.Errorf("google credentials changed despite env")
	}
}

func TestGoogleServiceAccountCanBeSetAndClearedFromAdmin(t *testing.T) {
	a := newAuthTestApp(t, appEnv{apiKey: "sk-env"})

	code, raw := a.do(t, http.MethodPut, "/api/agent/auth/google", googleBody(serviceAccountJSON(t, "ziggy@test-project.iam.gserviceaccount.com")))

	if code != http.StatusOK {
		t.Fatalf("PUT google = %d %s, want 200", code, raw)
	}
	if st := decodeStatus(t, raw); st.Google != (GoogleStatus{Set: true, ServiceAccountEmail: "ziggy@test-project.iam.gserviceaccount.com"}) {
		t.Errorf("google status = %+v, want set with the service account email", st.Google)
	}
	if !a.agent(t).CalendarWritable {
		t.Errorf("calendar not writable after adding a service account")
	}

	code, raw = a.do(t, http.MethodDelete, "/api/agent/auth/google", "")

	if code != http.StatusOK {
		t.Fatalf("DELETE google = %d %s, want 200", code, raw)
	}
	if st := decodeStatus(t, raw); st.Google != (GoogleStatus{}) {
		t.Errorf("google status = %+v, want cleared", st.Google)
	}
	if a.agent(t).CalendarWritable {
		t.Errorf("calendar still writable after removing the service account")
	}
}

func TestGoogleServiceAccountRejectsInvalidCredentials(t *testing.T) {
	a := newAuthTestApp(t, appEnv{})

	var missingEmail, badKey map[string]string
	json.Unmarshal([]byte(serviceAccountJSON(t, "x@test-project.iam.gserviceaccount.com")), &missingEmail)
	delete(missingEmail, "client_email")
	json.Unmarshal([]byte(serviceAccountJSON(t, "x@test-project.iam.gserviceaccount.com")), &badKey)
	badKey["private_key"] = "-----BEGIN PRIVATE KEY-----\nbm90IGEga2V5\n-----END PRIVATE KEY-----\n"
	encode := func(m map[string]string) string { b, _ := json.Marshal(m); return string(b) }
	for _, body := range []string{
		googleBody(""),
		googleBody("not json"),
		googleBody(`{"type":"authorized_user","client_id":"x"}`),
		googleBody(encode(missingEmail)),
		googleBody(encode(badKey)),
		`not json`,
	} {
		code, raw := a.do(t, http.MethodPut, "/api/agent/auth/google", body)
		if code != http.StatusBadRequest {
			t.Errorf("PUT %s = %d, want 400", body, code)
		}
		errorMessage(t, raw)
	}
	if a.status(t).Google.Set {
		t.Errorf("invalid credentials were stored")
	}
}

func TestGoogleServiceAccountFromEnvFilePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "google.json")
	if err := os.WriteFile(path, []byte(serviceAccountJSON(t, "file@test-project.iam.gserviceaccount.com")), 0o600); err != nil {
		t.Fatal(err)
	}
	a := newAuthTestApp(t, appEnv{apiKey: "sk-env", google: path})

	if st := a.status(t).Google; st != (GoogleStatus{Set: true, FromEnv: true, ServiceAccountEmail: "file@test-project.iam.gserviceaccount.com"}) {
		t.Errorf("google status = %+v, want the file's service account from env", st)
	}
	if !a.agent(t).CalendarWritable {
		t.Errorf("calendar not writable with a service account file from env")
	}
}

func TestChatGPTSignInWinsOverEnvAPIKey(t *testing.T) {
	a := newAuthTestApp(t, appEnv{apiKey: "sk-env"})

	if st := a.signIn(t); st.Using != "chatgpt" || !st.APIKey.FromEnv {
		t.Errorf("status = %+v, want ChatGPT used while the env key stays configured", st)
	}
	a.converse(t, "hello")
	if got := a.openai.authorizations(); len(got) != 1 || !strings.HasPrefix(got[0], "Bearer at-") {
		t.Errorf("Authorization = %v, want the ChatGPT access token", got)
	}
}

func TestChatGPTSignOutWarnsWhenRevocationIsNotConfirmed(t *testing.T) {
	a := newAuthTestApp(t, appEnv{})
	a.signIn(t)
	a.auth.Lock()
	a.auth.RevokeStatus = http.StatusInternalServerError
	a.auth.Unlock()

	code, raw := a.do(t, http.MethodDelete, "/api/agent/auth/chatgpt", "")

	if code != http.StatusOK {
		t.Fatalf("DELETE chatgpt = %d %s, want 200", code, raw)
	}
	st := decodeStatus(t, raw)
	if st.ChatGPT.SignedIn || strings.TrimSpace(st.Warning) == "" {
		t.Errorf("status = %+v, want signed out with a warning that revocation wasn't confirmed", st)
	}
	if _, raw := a.do(t, http.MethodGet, "/api/agent/auth", ""); bytes.Contains(raw, []byte(`"warning"`)) {
		t.Errorf("GET /api/agent/auth = %s, want no warning field outside the sign-out response", raw)
	}
}

func TestChatGPTSignInThroughEndpoints(t *testing.T) {
	a := newAuthTestApp(t, appEnv{})

	authURL := a.startSignIn(t)

	u, _ := url.Parse(authURL)
	if got := u.Scheme + "://" + u.Host + u.Path; got != a.auth.URL+"/api/accounts/authorize" {
		t.Errorf("authorizeUrl endpoint = %q", got)
	}
	if q := u.Query(); q.Get("client_id") != "dynamic_agent_client" || q.Get("redirect_uri") != "http://127.0.0.1:1455/auth/callback" {
		t.Errorf("authorizeUrl = %s, want a dynamic registration with the loopback callback", authURL)
	}

	code, raw := a.do(t, http.MethodPost, "/api/agent/auth/chatgpt/complete", completeBody(a.auth.Callback(authURL)))

	if code != http.StatusOK {
		t.Fatalf("complete = %d %s, want 200", code, raw)
	}
	st := decodeStatus(t, raw)
	if st.Using != "chatgpt" || st.ChatGPT != (ChatGPTStatus{SignedIn: true, Email: chatgpttest.Email}) {
		t.Errorf("status = %+v, want signed in to ChatGPT and using it", st)
	}
	if !a.agent(t).Ready {
		t.Errorf("agent not ready after signing in")
	}
	a.restart(t)
	if st := a.status(t); !st.ChatGPT.SignedIn {
		t.Errorf("status after restart = %+v, want still signed in", st)
	}
}

func TestChatGPTCompleteRejectsBadRedirects(t *testing.T) {
	cases := []struct {
		name     string
		redirect func(a *authTestApp, authURL string) string
		setup    func(a *authTestApp)
		wantText string
	}{
		{name: "unknown state", redirect: func(a *authTestApp, authURL string) string {
			return "http://127.0.0.1:1455/auth/callback?code=x&state=forged&client_id=oaiapp_x"
		}},
		{name: "declined", redirect: func(a *authTestApp, authURL string) string {
			return a.auth.ErrorCallback(authURL, "access_denied", "You declined to share your ChatGPT plan.")
		}, wantText: "You declined to share your ChatGPT plan."},
		{name: "exchange fails", setup: func(a *authTestApp) {
			a.auth.Lock()
			a.auth.TokenError = "invalid_grant"
			a.auth.TokenErrorDescription = "Authorization code expired."
			a.auth.Unlock()
		}, redirect: func(a *authTestApp, authURL string) string { return a.auth.Callback(authURL) }, wantText: "Authorization code expired."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := newAuthTestApp(t, appEnv{})
			if tc.setup != nil {
				tc.setup(a)
			}
			authURL := a.startSignIn(t)

			code, raw := a.do(t, http.MethodPost, "/api/agent/auth/chatgpt/complete", completeBody(tc.redirect(a, authURL)))

			if code != http.StatusBadRequest {
				t.Errorf("complete = %d %s, want 400", code, raw)
			}
			if msg := errorMessage(t, raw); tc.wantText != "" && !strings.Contains(msg, tc.wantText) {
				t.Errorf("error = %q, want it to include %q", msg, tc.wantText)
			}
			if a.status(t).ChatGPT.SignedIn {
				t.Errorf("signed in after a rejected redirect")
			}
		})
	}
	t.Run("malformed body", func(t *testing.T) {
		a := newAuthTestApp(t, appEnv{})
		for _, body := range []string{`not json`, `{}`} {
			code, raw := a.do(t, http.MethodPost, "/api/agent/auth/chatgpt/complete", body)
			if code != http.StatusBadRequest {
				t.Errorf("complete %s = %d, want 400", body, code)
			}
			errorMessage(t, raw)
		}
	})
}

func TestChatGPTSignOutFallsBackToAPIKey(t *testing.T) {
	a := newAuthTestApp(t, appEnv{})
	a.do(t, http.MethodPut, "/api/agent/auth/api-key", `{"key":"sk-admin"}`)
	if st := a.signIn(t); st.Using != "chatgpt" {
		t.Fatalf("status after sign-in = %+v, want using chatgpt", st)
	}

	code, raw := a.do(t, http.MethodDelete, "/api/agent/auth/chatgpt", "")

	if code != http.StatusOK {
		t.Fatalf("DELETE chatgpt = %d %s, want 200", code, raw)
	}
	if st := decodeStatus(t, raw); st.Using != "apiKey" || st.ChatGPT.SignedIn || !st.APIKey.Set {
		t.Errorf("status = %+v, want signed out and back on the api key", st)
	}
	if n := len(a.auth.Revocations()); n != 1 {
		t.Errorf("revocations = %d, want the refresh token revoked", n)
	}
	if bytes.Contains(raw, []byte(`"warning"`)) {
		t.Errorf("sign-out response %s has a warning after a confirmed revocation", raw)
	}
	// Signing in again reuses the issued client.
	if q, _ := url.Parse(a.startSignIn(t)); q.Query().Get("client_id") != chatgpttest.IssuedClientID {
		t.Errorf("sign-in after sign-out client_id = %q, want the issued id", q.Query().Get("client_id"))
	}
}

func TestAgentUsesChatGPTWhenSignedInElseAPIKey(t *testing.T) {
	a := newAuthTestApp(t, appEnv{})
	a.do(t, http.MethodPut, "/api/agent/auth/api-key", `{"key":"sk-admin"}`)

	a.converse(t, "one")
	a.signIn(t)
	a.converse(t, "two")
	a.do(t, http.MethodDelete, "/api/agent/auth/chatgpt", "")
	a.converse(t, "three")

	got := a.openai.authorizations()
	if len(got) != 3 || got[0] != "Bearer sk-admin" || got[2] != "Bearer sk-admin" {
		t.Fatalf("Authorization headers = %v, want api key, ChatGPT token, api key", got)
	}
	if !strings.HasPrefix(got[1], "Bearer at-") {
		t.Errorf("signed-in request Authorization = %q, want the ChatGPT access token", got[1])
	}
}

func TestCredentialChangesTakeEffectWithoutRestart(t *testing.T) {
	a := newAuthTestApp(t, appEnv{})
	if snap := a.agent(t); snap.Ready || snap.CalendarWritable {
		t.Fatalf("agent = %+v, want not ready and read-only to start", snap)
	}

	a.do(t, http.MethodPut, "/api/agent/auth/api-key", `{"key":"sk-admin"}`)
	a.nextAgentFrame(t, func(s types.AgentSnapshot) bool { return s.Ready })
	if !a.agent(t).Ready {
		t.Errorf("GET /api/agent not ready after setting a key")
	}
	if out := a.converse(t, "add lunch tomorrow"); out != "Okay." {
		t.Errorf("converse = %q, want the model's reply", out)
	}
	if offered(a.openai.lastTools(), "create_event") {
		t.Errorf("create_event offered without Google credentials")
	}

	a.do(t, http.MethodPut, "/api/agent/auth/google", googleBody(serviceAccountJSON(t, "ziggy@test-project.iam.gserviceaccount.com")))
	a.nextAgentFrame(t, func(s types.AgentSnapshot) bool { return s.CalendarWritable })
	a.converse(t, "add lunch tomorrow")
	if !offered(a.openai.lastTools(), "create_event") {
		t.Errorf("tools = %v, want create_event offered once Google credentials are added", a.openai.lastTools())
	}

	a.do(t, http.MethodDelete, "/api/agent/auth/google", "")
	a.nextAgentFrame(t, func(s types.AgentSnapshot) bool { return !s.CalendarWritable })
	a.converse(t, "add lunch tomorrow")
	if offered(a.openai.lastTools(), "create_event") {
		t.Errorf("create_event still offered after removing Google credentials")
	}

	a.do(t, http.MethodDelete, "/api/agent/auth/api-key", "")
	a.nextAgentFrame(t, func(s types.AgentSnapshot) bool { return !s.Ready })
}

func offered(names []string, name string) bool {
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}

func TestRevokedChatGPTSignInMakesZiggyAskToSignInAgain(t *testing.T) {
	// One-second tokens are always due for refresh.
	a := newAuthTestApp(t, appEnv{expiresIn: 1})
	a.signIn(t)
	a.auth.Lock()
	a.auth.RefreshError = "invalid_grant"
	a.auth.Unlock()

	out := a.converse(t, "hello")

	if lower := strings.ToLower(out); !strings.Contains(lower, "sign in again") || !strings.Contains(lower, "calendar settings") {
		t.Errorf("output = %q, want Ziggy to ask for signing in again in the calendar settings", out)
	}
	if n := a.openai.hits.Load(); n != 0 {
		t.Errorf("OpenAI hits = %d, want none with a revoked sign-in", n)
	}
	if st := a.status(t); st.ChatGPT.SignedIn || st.Using != "none" {
		t.Errorf("status = %+v, want signed out", st)
	}
	a.nextAgentFrame(t, func(s types.AgentSnapshot) bool { return !s.Ready })
}

func TestSecretsNeverLeaveTheServer(t *testing.T) {
	a := newAuthTestApp(t, appEnv{})
	sa := serviceAccountJSON(t, "ziggy@test-project.iam.gserviceaccount.com")
	var saFields struct {
		PrivateKey string `json:"private_key"`
	}
	json.Unmarshal([]byte(sa), &saFields)
	keyLine := strings.Split(saFields.PrivateKey, "\n")[1]

	a.do(t, http.MethodPut, "/api/agent/auth/api-key", `{"key":"sk-very-secret-key"}`)
	a.do(t, http.MethodPut, "/api/agent/auth/google", googleBody(sa))
	a.signIn(t)
	a.converse(t, "hello")

	tokens := a.auth.IssuedTokens()
	if len(tokens) < 3 {
		t.Fatalf("issued %d ChatGPT tokens, want access, refresh and ID tokens", len(tokens))
	}
	secretValues := append([]string{"sk-very-secret-key", keyLine}, tokens...)

	var exposed []string
	check := func(where string, data []byte) {
		for _, secret := range secretValues {
			if bytes.Contains(data, []byte(secret)) {
				exposed = append(exposed, where)
			}
		}
	}
	for _, path := range []string{"/api/config", "/api/agent", "/api/agent/auth"} {
		_, raw := a.do(t, http.MethodGet, path, "")
		check("GET "+path, raw)
	}
	_, cfgRaw := a.do(t, http.MethodGet, "/api/config", "")
	_, raw := a.do(t, http.MethodPut, "/api/config", string(cfgRaw))
	check("PUT /api/config", raw)
	conn, ctx := dialWS(t, a.app)
	_, snap, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read ws snapshot: %v", err)
	}
	check("WS snapshot", snap)
	configFile, _ := os.ReadFile(filepath.Join(a.dir, "config.json"))
	check("config.json", configFile)
	if len(exposed) > 0 {
		t.Errorf("secrets exposed in %v", exposed)
	}
	info, _ := os.Stat(filepath.Join(a.dir, "secrets.json"))
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("secrets.json mode = %o, want 600", perm)
	}
}

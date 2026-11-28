package chatgpt_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jpatters/home-calendar/internal/chatgpt"
	"github.com/jpatters/home-calendar/internal/chatgpt/chatgpttest"
	"github.com/jpatters/home-calendar/internal/secrets"
)

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

type harness struct {
	auth    *chatgpttest.Server
	client  *chatgpt.Client
	clk     *clock
	dir     string
	mu      sync.Mutex
	changes int
}

func (h *harness) changed() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.changes
}

func (h *harness) secrets(t *testing.T) secrets.ChatGPT {
	t.Helper()
	s, err := secrets.Open(filepath.Join(h.dir, "secrets.json"))
	if err != nil {
		t.Fatalf("reopen secrets: %v", err)
	}
	return s.ChatGPT()
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	t.Setenv(secrets.EnvOpenAIKey, "")
	t.Setenv(secrets.EnvGoogleServiceAccount, "")
	h := &harness{auth: chatgpttest.New(t), clk: &clock{now: time.Now()}, dir: t.TempDir()}
	h.auth.Lock()
	h.auth.Now = h.clk.Now
	h.auth.Unlock()
	h.client = h.newClient(t)
	return h
}

// newClient builds a client over the harness's secrets file, as a restarted
// server would.
func (h *harness) newClient(t *testing.T) *chatgpt.Client {
	t.Helper()
	store, err := secrets.Open(filepath.Join(h.dir, "secrets.json"))
	if err != nil {
		t.Fatalf("secrets.Open: %v", err)
	}
	return chatgpt.New(chatgpt.Options{
		Issuer:  h.auth.URL,
		Secrets: store,
		Now:     h.clk.Now,
		OnChange: func() {
			h.mu.Lock()
			h.changes++
			h.mu.Unlock()
		},
	})
}

func start(t *testing.T, c *chatgpt.Client) (string, url.Values) {
	t.Helper()
	raw, err := c.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("authorize URL %q: %v", raw, err)
	}
	return raw, u.Query()
}

func signIn(t *testing.T, h *harness) {
	t.Helper()
	authURL, _ := start(t, h.client)
	if err := h.client.Complete(context.Background(), h.auth.Callback(authURL)); err != nil {
		t.Fatalf("Complete: %v", err)
	}
}

func lastTokenRequest(t *testing.T, a *chatgpttest.Server) url.Values {
	t.Helper()
	reqs := a.TokenRequests()
	if len(reqs) == 0 {
		t.Fatalf("no token endpoint requests")
	}
	return reqs[len(reqs)-1]
}

func TestFirstSignInRequestsDynamicRegistrationWithPKCE(t *testing.T) {
	h := newHarness(t)

	raw, q := start(t, h.client)

	u, _ := url.Parse(raw)
	if got := u.Scheme + "://" + u.Host + u.Path; got != h.auth.URL+"/api/accounts/authorize" {
		t.Errorf("authorize endpoint = %q, want %s/api/accounts/authorize", got, h.auth.URL)
	}
	want := map[string]string{
		"client_id":             "dynamic_agent_client",
		"agent_name_hint":       chatgpt.AppName,
		"response_type":         "code",
		"redirect_uri":          "http://127.0.0.1:1455/auth/callback",
		"scope":                 "openid profile email offline_access resource.invoke chatgpt.tokens.use.direct",
		"resource":              "https://api.openai.com/v1",
		"code_challenge_method": "S256",
		"ext_agent_host_id":     h.secrets(t).HostID,
	}
	for k, v := range want {
		if q.Get(k) != v {
			t.Errorf("%s = %q, want %q", k, q.Get(k), v)
		}
	}
	if !strings.HasPrefix(q.Get("ext_agent_host_id"), "urn:uuid:") {
		t.Errorf("ext_agent_host_id = %q, want the persisted urn:uuid host id", q.Get("ext_agent_host_id"))
	}
	for _, k := range []string{"state", "nonce"} {
		if len(q.Get(k)) < 16 {
			t.Errorf("%s = %q, want a fresh random value", k, q.Get(k))
		}
	}
	if !regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`).MatchString(q.Get("code_challenge")) {
		t.Errorf("code_challenge = %q, want an unpadded base64url SHA-256 digest", q.Get("code_challenge"))
	}
	for _, k := range []string{"id_token_hint", "login_hint", "client_secret"} {
		if q.Has(k) {
			t.Errorf("first sign-in sent %s", k)
		}
	}
}

func TestEachAttemptGetsFreshStateNonceAndVerifier(t *testing.T) {
	h := newHarness(t)

	_, a := start(t, h.client)
	_, b := start(t, h.client)

	for _, k := range []string{"state", "nonce", "code_challenge"} {
		if a.Get(k) == b.Get(k) {
			t.Errorf("%s repeated across attempts: %q", k, a.Get(k))
		}
	}
	if a.Get("ext_agent_host_id") != b.Get("ext_agent_host_id") {
		t.Errorf("host id changed between attempts")
	}
}

func TestCompleteExchangesCodeWithIssuedClientIDAndSavesCredentials(t *testing.T) {
	h := newHarness(t)
	authURL, q := start(t, h.client)

	if err := h.client.Complete(context.Background(), h.auth.Callback(authURL)); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	form := lastTokenRequest(t, h.auth)
	sum := sha256.Sum256([]byte(form.Get("code_verifier")))
	if base64.RawURLEncoding.EncodeToString(sum[:]) != q.Get("code_challenge") {
		t.Errorf("code_verifier does not match the code_challenge sent")
	}
	for k, v := range map[string]string{
		"grant_type":   "authorization_code",
		"client_id":    chatgpttest.IssuedClientID,
		"redirect_uri": "http://127.0.0.1:1455/auth/callback",
		"resource":     "https://api.openai.com/v1",
	} {
		if form.Get(k) != v {
			t.Errorf("token request %s = %q, want %q", k, form.Get(k), v)
		}
	}
	if form.Has("client_secret") {
		t.Errorf("token request sent a client_secret")
	}
	if got := h.client.Status(); !got.SignedIn || got.Email != chatgpttest.Email {
		t.Errorf("Status = %+v, want signed in as %s", got, chatgpttest.Email)
	}
	saved := h.secrets(t)
	if saved.ClientID != chatgpttest.IssuedClientID || saved.AccessToken == "" || saved.RefreshToken == "" || saved.IDToken == "" {
		t.Errorf("saved credentials = %+v, want issued client id and tokens", saved)
	}
	if !saved.ExpiresAt.After(h.clk.Now().Add(50 * time.Minute)) {
		t.Errorf("saved ExpiresAt = %v, want about an hour from now", saved.ExpiresAt)
	}
	if h.changed() == 0 {
		t.Errorf("OnChange not called after sign-in")
	}
	if got := h.newClient(t).Status(); !got.SignedIn {
		t.Errorf("Status after restart = %+v, want still signed in", got)
	}
}

func TestSignInAgainReusesIssuedClientIDAndHostID(t *testing.T) {
	h := newHarness(t)
	_, first := start(t, h.client)
	signIn(t, h)
	if err := h.client.SignOut(context.Background()); err != nil {
		t.Fatalf("SignOut: %v", err)
	}

	authURL, q := start(t, h.client)

	if q.Get("client_id") != chatgpttest.IssuedClientID {
		t.Errorf("reauthorization client_id = %q, want the issued %q", q.Get("client_id"), chatgpttest.IssuedClientID)
	}
	if q.Has("agent_name_hint") {
		t.Errorf("reauthorization sent agent_name_hint")
	}
	if q.Get("ext_agent_host_id") != first.Get("ext_agent_host_id") {
		t.Errorf("host id changed: %q -> %q", first.Get("ext_agent_host_id"), q.Get("ext_agent_host_id"))
	}
	if q.Get("login_hint") != chatgpttest.Email {
		t.Errorf("login_hint = %q, want the saved email", q.Get("login_hint"))
	}
	if q.Has("id_token_hint") {
		t.Errorf("id_token_hint sent after signing out")
	}
	// A reauthorization callback may omit client_id.
	if err := h.client.Complete(context.Background(), h.auth.CallbackWithoutClientID(authURL)); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got := lastTokenRequest(t, h.auth).Get("client_id"); got != chatgpttest.IssuedClientID {
		t.Errorf("token exchange client_id = %q, want the issued id", got)
	}
}

func TestSignInWhileSignedInSendsIDTokenHint(t *testing.T) {
	h := newHarness(t)
	signIn(t, h)

	_, q := start(t, h.client)

	if q.Get("id_token_hint") != h.secrets(t).IDToken {
		t.Errorf("id_token_hint = %q, want the retained ID token", q.Get("id_token_hint"))
	}
}

func TestCompleteRejectsBadCallbacks(t *testing.T) {
	cases := []struct {
		name     string
		callback func(h *harness, authURL string) string
		wantText string
	}{
		{"unknown state", func(h *harness, authURL string) string {
			cb := h.auth.Callback(authURL)
			u, _ := url.Parse(cb)
			q := u.Query()
			q.Set("state", "not-the-state")
			u.RawQuery = q.Encode()
			return u.String()
		}, ""},
		{"declined consent", func(h *harness, authURL string) string {
			return h.auth.ErrorCallback(authURL, "access_denied", "The user declined access.")
		}, "The user declined access."},
		{"registration without issued client id", func(h *harness, authURL string) string {
			return h.auth.CallbackWithoutClientID(authURL)
		}, ""},
		{"not a URL", func(h *harness, authURL string) string { return "%%%" }, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			authURL, _ := start(t, h.client)

			err := h.client.Complete(context.Background(), tc.callback(h, authURL))

			if err == nil {
				t.Fatalf("Complete err = nil, want rejection")
			}
			if tc.wantText != "" && !strings.Contains(err.Error(), tc.wantText) {
				t.Errorf("err = %q, want it to include %q", err, tc.wantText)
			}
			if n := len(h.auth.TokenRequests()); n != 0 {
				t.Errorf("token requests = %d, want no code exchange", n)
			}
			if h.client.Status().SignedIn {
				t.Errorf("signed in after a rejected callback")
			}
		})
	}
}

func TestCompleteRejectsExpiredAttempt(t *testing.T) {
	h := newHarness(t)
	authURL, _ := start(t, h.client)
	h.clk.Advance(11 * time.Minute)

	if err := h.client.Complete(context.Background(), h.auth.Callback(authURL)); err == nil {
		t.Fatalf("Complete err = nil, want the expired attempt rejected")
	}
	if n := len(h.auth.TokenRequests()); n != 0 {
		t.Errorf("token requests = %d, want no code exchange", n)
	}
}

func TestCompleteAttemptIsSingleUse(t *testing.T) {
	h := newHarness(t)
	authURL, _ := start(t, h.client)
	cb := h.auth.Callback(authURL)
	if err := h.client.Complete(context.Background(), cb); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if err := h.client.Complete(context.Background(), cb); err == nil {
		t.Errorf("second Complete with the same state err = nil, want rejection")
	}
}

func TestCompleteReportsTokenEndpointError(t *testing.T) {
	h := newHarness(t)
	h.auth.Lock()
	h.auth.TokenError = "invalid_grant"
	h.auth.TokenErrorDescription = "The authorization code has expired."
	h.auth.Unlock()
	authURL, _ := start(t, h.client)

	err := h.client.Complete(context.Background(), h.auth.Callback(authURL))

	if err == nil || !strings.Contains(err.Error(), "The authorization code has expired.") {
		t.Errorf("err = %v, want OpenAI's error description", err)
	}
	if h.client.Status().SignedIn {
		t.Errorf("signed in after a failed exchange")
	}
}

func TestCompleteRejectsReauthorizationNamingAnotherClient(t *testing.T) {
	h := newHarness(t)
	signIn(t, h)
	h.client.SignOut(context.Background())
	authURL, _ := start(t, h.client)

	err := h.client.Complete(context.Background(), h.auth.CallbackWithClientID(authURL, "oaiapp_someone_else"))

	if err == nil {
		t.Fatalf("Complete err = nil, want a different client id rejected")
	}
	if got := h.secrets(t).ClientID; got != chatgpttest.IssuedClientID {
		t.Errorf("saved client id = %q, want the original registration kept", got)
	}
}

func TestCompleteRejectsInvalidIDTokens(t *testing.T) {
	claims := func(edit func(h *harness, c map[string]any)) func(*testing.T, *harness) {
		return func(t *testing.T, h *harness) {
			h.auth.Lock()
			h.auth.IDTokenClaims = func(c map[string]any) { edit(h, c) }
			h.auth.Unlock()
		}
	}
	header := func(edit func(map[string]any)) func(*testing.T, *harness) {
		return func(t *testing.T, h *harness) {
			h.auth.Lock()
			h.auth.IDTokenHeader = edit
			h.auth.Unlock()
		}
	}
	cases := []struct {
		name  string
		setup func(*testing.T, *harness)
	}{
		{"forged signature", func(t *testing.T, h *harness) { h.auth.ForgeIDTokenSignature(t) }},
		{"unsigned (alg none)", header(func(hd map[string]any) { hd["alg"] = "none" })},
		{"unknown key id", header(func(hd map[string]any) { hd["kid"] = "rotated-away" })},
		{"wrong nonce", claims(func(h *harness, c map[string]any) { c["nonce"] = "replayed" })},
		{"wrong audience", claims(func(h *harness, c map[string]any) { c["aud"] = "oaiapp_other" })},
		{"wrong issuer", claims(func(h *harness, c map[string]any) { c["iss"] = "https://evil.example" })},
		{"expired", claims(func(h *harness, c map[string]any) { c["exp"] = h.clk.Now().Add(-time.Hour).Unix() })},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			tc.setup(t, h)
			authURL, _ := start(t, h.client)

			if err := h.client.Complete(context.Background(), h.auth.Callback(authURL)); err == nil {
				t.Fatalf("Complete err = nil, want the ID token rejected")
			}
			if h.client.Status().SignedIn || h.secrets(t).AccessToken != "" {
				t.Errorf("credentials saved despite an invalid ID token")
			}
		})
	}
}

func TestCompleteRequiresChatGPTPlanScope(t *testing.T) {
	h := newHarness(t)
	h.auth.Lock()
	h.auth.Scope = "openid profile email offline_access resource.invoke"
	h.auth.Unlock()
	authURL, _ := start(t, h.client)

	err := h.client.Complete(context.Background(), h.auth.Callback(authURL))

	if err == nil {
		t.Fatalf("Complete err = nil, want an error when ChatGPT plan use was not granted")
	}
	if tok, _ := h.client.AccessToken(context.Background()); tok != "" {
		t.Errorf("AccessToken = %q, want none without plan permission", tok)
	}
}

func TestCompleteRejectsDifferentAccountOnReauthorization(t *testing.T) {
	h := newHarness(t)
	signIn(t, h)
	h.client.SignOut(context.Background())
	h.auth.Lock()
	h.auth.Subject = "someone-else"
	h.auth.Unlock()
	authURL, _ := start(t, h.client)

	if err := h.client.Complete(context.Background(), h.auth.CallbackWithoutClientID(authURL)); err == nil {
		t.Errorf("Complete err = nil, want a different account rejected for this registration")
	}
}

func TestAccessTokenUsesSavedTokenUntilShortlyBeforeExpiry(t *testing.T) {
	h := newHarness(t)
	signIn(t, h)
	saved := h.secrets(t)
	h.clk.Advance(50 * time.Minute)

	tok, err := h.client.AccessToken(context.Background())

	if err != nil || tok != saved.AccessToken {
		t.Errorf("AccessToken = %q, %v; want the saved %q", tok, err, saved.AccessToken)
	}
	if n := h.auth.Refreshes(); n != 0 {
		t.Errorf("refreshes = %d, want 0 while the token is fresh", n)
	}
}

func TestAccessTokenRefreshesNearExpiryAndPersistsRotation(t *testing.T) {
	h := newHarness(t)
	signIn(t, h)
	saved := h.secrets(t)
	h.clk.Advance(58 * time.Minute)

	tok, err := h.client.AccessToken(context.Background())

	if err != nil {
		t.Fatalf("AccessToken: %v", err)
	}
	if tok == saved.AccessToken {
		t.Errorf("AccessToken returned the expiring token, want a refreshed one")
	}
	form := lastTokenRequest(t, h.auth)
	for k, v := range map[string]string{
		"grant_type":    "refresh_token",
		"client_id":     chatgpttest.IssuedClientID,
		"refresh_token": saved.RefreshToken,
		"resource":      "https://api.openai.com/v1",
	} {
		if form.Get(k) != v {
			t.Errorf("refresh request %s = %q, want %q", k, form.Get(k), v)
		}
	}
	if form.Has("scope") {
		t.Errorf("refresh request sent scope %q, want it omitted", form.Get("scope"))
	}
	after := h.secrets(t)
	if after.AccessToken != tok || after.RefreshToken == saved.RefreshToken || after.RefreshToken == "" {
		t.Errorf("saved after refresh = access %q refresh %q, want the rotated pair", after.AccessToken, after.RefreshToken)
	}
	if !after.ExpiresAt.After(h.clk.Now().Add(50 * time.Minute)) {
		t.Errorf("saved ExpiresAt = %v, want renewed", after.ExpiresAt)
	}
	// A restarted server uses the rotated refresh token.
	h.clk.Advance(58 * time.Minute)
	if _, err := h.newClient(t).AccessToken(context.Background()); err != nil {
		t.Errorf("refresh after restart: %v", err)
	}
}

func TestRefreshKeepsTheNewIDTokenForTheNextSignIn(t *testing.T) {
	h := newHarness(t)
	signIn(t, h)
	before := h.secrets(t).IDToken
	h.auth.Lock()
	h.auth.RefreshIDToken = true
	h.auth.Unlock()
	h.clk.Advance(58 * time.Minute)

	if _, err := h.client.AccessToken(context.Background()); err != nil {
		t.Fatalf("AccessToken: %v", err)
	}

	after := h.secrets(t).IDToken
	if after == "" || after == before {
		t.Errorf("saved ID token after refresh = %q, want the newly issued one", after)
	}
	if _, q := start(t, h.client); q.Get("id_token_hint") != after {
		t.Errorf("id_token_hint = %q, want the refreshed ID token", q.Get("id_token_hint"))
	}
}

func TestConcurrentAccessTokenCallsShareOneRefresh(t *testing.T) {
	h := newHarness(t)
	signIn(t, h)
	h.auth.Lock()
	h.auth.RefreshDelay = 100 * time.Millisecond
	h.auth.Unlock()
	h.clk.Advance(59 * time.Minute)

	var wg sync.WaitGroup
	toks := make([]string, 20)
	errs := make([]error, 20)
	for i := range toks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			toks[i], errs[i] = h.client.AccessToken(context.Background())
		}()
	}
	wg.Wait()

	if n := h.auth.Refreshes(); n != 1 {
		t.Errorf("refreshes = %d, want exactly 1", n)
	}
	for i := range toks {
		if errs[i] != nil || toks[i] != toks[0] || toks[i] == "" {
			t.Errorf("call %d = %q, %v; want the same refreshed token as call 0 (%q)", i, toks[i], errs[i], toks[0])
		}
	}
}

func TestRevokedRefreshTokenSignsOut(t *testing.T) {
	for _, code := range []string{"invalid_grant", "refresh_token_expired", "refresh_token_reused", "refresh_token_invalidated", "invalid_refresh_token", "token_expired"} {
		t.Run(code, func(t *testing.T) {
			h := newHarness(t)
			signIn(t, h)
			before := h.changed()
			h.auth.Lock()
			h.auth.RefreshError = code
			h.auth.Unlock()
			h.clk.Advance(59 * time.Minute)

			tok, err := h.client.AccessToken(context.Background())

			if !errors.Is(err, chatgpt.ErrSignedOut) || tok != "" {
				t.Errorf("AccessToken = %q, %v; want ErrSignedOut", tok, err)
			}
			if h.client.Status().SignedIn {
				t.Errorf("still signed in after a revoked refresh token")
			}
			saved := h.secrets(t)
			if saved.AccessToken != "" || saved.RefreshToken != "" || saved.IDToken != "" {
				t.Errorf("tokens kept after revocation: %+v", saved)
			}
			if saved.ClientID != chatgpttest.IssuedClientID || saved.HostID == "" {
				t.Errorf("registration lost: client %q host %q", saved.ClientID, saved.HostID)
			}
			if h.changed() == before {
				t.Errorf("OnChange not called when the sign-in was lost")
			}
			if tok, err := h.client.AccessToken(context.Background()); tok != "" || err != nil {
				t.Errorf("AccessToken after sign-out = %q, %v; want \"\", nil", tok, err)
			}
		})
	}
}

func TestTransientRefreshFailureKeepsCredentials(t *testing.T) {
	h := newHarness(t)
	signIn(t, h)
	h.auth.Lock()
	h.auth.RefreshStatus = 503
	h.auth.Unlock()
	h.clk.Advance(59 * time.Minute)

	_, err := h.client.AccessToken(context.Background())

	if err == nil || errors.Is(err, chatgpt.ErrSignedOut) {
		t.Errorf("err = %v, want a non-sign-out failure", err)
	}
	if !h.client.Status().SignedIn || h.secrets(t).RefreshToken == "" {
		t.Errorf("credentials dropped after a temporary failure")
	}
}

func TestSignOutRevokesRefreshTokenAndForgetsTokens(t *testing.T) {
	h := newHarness(t)
	signIn(t, h)
	saved := h.secrets(t)
	before := h.changed()

	if err := h.client.SignOut(context.Background()); err != nil {
		t.Fatalf("SignOut: %v", err)
	}

	revs := h.auth.Revocations()
	if len(revs) != 1 {
		t.Fatalf("revocations = %d, want 1", len(revs))
	}
	for k, v := range map[string]string{"token": saved.RefreshToken, "token_type_hint": "refresh_token", "client_id": chatgpttest.IssuedClientID} {
		if revs[0].Get(k) != v {
			t.Errorf("revocation %s = %q, want %q", k, revs[0].Get(k), v)
		}
	}
	after := h.secrets(t)
	if after.AccessToken != "" || after.RefreshToken != "" || after.IDToken != "" {
		t.Errorf("tokens kept after sign-out: %+v", after)
	}
	if after.ClientID != saved.ClientID || after.HostID != saved.HostID {
		t.Errorf("registration changed on sign-out: %+v", after)
	}
	if h.client.Status().SignedIn || h.changed() == before {
		t.Errorf("Status = %+v, OnChange calls %d->%d; want signed out and notified", h.client.Status(), before, h.changed())
	}
}

func TestSignOutForgetsTokensEvenWhenRevocationFails(t *testing.T) {
	h := newHarness(t)
	signIn(t, h)
	h.auth.Lock()
	h.auth.RevokeStatus = 500
	h.auth.Unlock()

	err := h.client.SignOut(context.Background())

	if err == nil {
		t.Errorf("SignOut err = nil, want unconfirmed revocation reported")
	}
	if n := len(h.auth.Revocations()); n < 2 {
		t.Errorf("revocation attempts = %d, want retries after a server error", n)
	}
	if h.client.Status().SignedIn || h.secrets(t).RefreshToken != "" {
		t.Errorf("still holding tokens after sign-out with failed revocation")
	}
}

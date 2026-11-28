// Package chatgpt implements OpenAI's "Sign in with ChatGPT" flow for
// open-source, locally hosted apps: dynamic client registration, the
// authorization-code + PKCE (S256) grant, ID-token validation, rotating
// refresh tokens and revocation. The resulting access token is used as the
// bearer credential for the Responses API on the user's ChatGPT plan.
package chatgpt

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jpatters/home-calendar/internal/secrets"
)

const (
	// DefaultIssuer is OpenAI's authorization server.
	DefaultIssuer = "https://auth.openai.com"
	// RedirectURI is the loopback callback registered for this app. Nothing
	// listens on it: the user pastes the URL the browser lands on into the
	// calendar's settings.
	RedirectURI = "http://127.0.0.1:1455/auth/callback"
	// Resource is the API the tokens are issued for.
	Resource = "https://api.openai.com/v1"
	// Scope is every scope requested: identity plus ChatGPT plan usage.
	Scope = "openid profile email offline_access resource.invoke chatgpt.tokens.use.direct"
	// AppName is sent as agent_name_hint on first registration.
	AppName = "Home Calendar"

	dynamicClientID = "dynamic_agent_client"
	planScope       = "chatgpt.tokens.use.direct"
	// attemptTTL is how long a started sign-in can be completed.
	attemptTTL = 10 * time.Minute
	// refreshMargin is how long before expiry an access token is renewed.
	refreshMargin = 5 * time.Minute
)

// ErrSignedOut reports that the saved sign-in can no longer be refreshed
// (revoked, expired or reused refresh token); the user must sign in again.
var ErrSignedOut = errors.New("chatgpt: signed out; sign in again")

// terminalRefreshErrors are the refresh error codes after which the refresh
// token is unusable and OAuth must be repeated.
var terminalRefreshErrors = map[string]bool{
	"invalid_grant":             true,
	"invalid_refresh_token":     true,
	"token_expired":             true,
	"refresh_token_expired":     true,
	"refresh_token_invalidated": true,
	"refresh_token_reused":      true,
}

type Options struct {
	// Issuer is the authorization server; empty uses DefaultIssuer.
	Issuer     string
	Secrets    *secrets.Store
	HTTPClient *http.Client
	Now        func() time.Time
	// OnChange is called after the signed-in state changes (sign-in, sign
	// out, or a refresh that found the sign-in revoked).
	OnChange func()
}

// Status is the sign-in state shown in the calendar's settings.
type Status struct {
	SignedIn bool
	Email    string
}

type attempt struct {
	clientID string
	verifier string
	nonce    string
	started  time.Time
}

type Client struct {
	o Options

	mu       sync.Mutex
	attempts map[string]attempt

	// tokenMu serializes refreshes and sign-out, so a rotating refresh
	// token is never used twice.
	tokenMu sync.Mutex
}

func New(o Options) *Client {
	if o.Issuer == "" {
		o.Issuer = DefaultIssuer
	}
	o.Issuer = strings.TrimRight(o.Issuer, "/")
	if o.HTTPClient == nil {
		o.HTTPClient = &http.Client{Timeout: 15 * time.Second}
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.OnChange == nil {
		o.OnChange = func() {}
	}
	return &Client{o: o, attempts: map[string]attempt{}}
}

func (c *Client) Status() Status {
	reg := c.o.Secrets.ChatGPT()
	if reg.AccessToken == "" {
		return Status{}
	}
	return Status{SignedIn: true, Email: reg.Email}
}

// Start begins a sign-in attempt and returns the URL to open in a browser.
// The attempt's state, nonce and PKCE verifier are kept in memory for ten
// minutes.
func (c *Client) Start() (string, error) {
	reg := c.o.Secrets.ChatGPT()
	state, nonce, verifier := randomString(), randomString(), randomString()
	sum := sha256.Sum256([]byte(verifier))
	q := url.Values{
		"response_type":         {"code"},
		"redirect_uri":          {RedirectURI},
		"scope":                 {Scope},
		"resource":              {Resource},
		"state":                 {state},
		"nonce":                 {nonce},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(sum[:])},
		"code_challenge_method": {"S256"},
		"ext_agent_host_id":     {reg.HostID},
	}
	clientID := reg.ClientID
	if clientID == "" {
		clientID = dynamicClientID
		q.Set("agent_name_hint", AppName)
	} else {
		if reg.IDToken != "" {
			q.Set("id_token_hint", reg.IDToken)
		}
		if reg.Email != "" {
			q.Set("login_hint", reg.Email)
		}
	}
	q.Set("client_id", clientID)

	now := c.o.Now()
	c.mu.Lock()
	for s, a := range c.attempts {
		if now.Sub(a.started) > attemptTTL {
			delete(c.attempts, s)
		}
	}
	c.attempts[state] = attempt{clientID: clientID, verifier: verifier, nonce: nonce, started: now}
	c.mu.Unlock()
	return c.o.Issuer + "/api/accounts/authorize?" + q.Encode(), nil
}

// Complete finishes the attempt named by the state in redirectURL (the URL
// the browser landed on): it validates the callback, exchanges the code,
// validates the ID token and granted scopes, and saves the credentials.
func (c *Client) Complete(ctx context.Context, redirectURL string) error {
	u, err := url.Parse(strings.TrimSpace(redirectURL))
	if err != nil {
		return errors.New("that isn't the address the sign-in page sent you to")
	}
	q := u.Query()
	c.mu.Lock()
	att, ok := c.attempts[q.Get("state")]
	delete(c.attempts, q.Get("state"))
	c.mu.Unlock()
	if !ok || q.Get("state") == "" || c.o.Now().Sub(att.started) > attemptTTL {
		return errors.New("this sign-in has expired or was already used; start again")
	}
	if e := q.Get("error"); e != "" {
		if d := q.Get("error_description"); d != "" {
			return fmt.Errorf("ChatGPT sign-in failed: %s", d)
		}
		return fmt.Errorf("ChatGPT sign-in failed: %s", e)
	}
	code := q.Get("code")
	if code == "" {
		return errors.New("the address has no authorization code; start again")
	}
	clientID := att.clientID
	issued := q.Get("client_id")
	if clientID == dynamicClientID {
		if issued == "" || issued == dynamicClientID {
			return errors.New("ChatGPT didn't finish registering this app; start again")
		}
		clientID = issued
	} else if issued != "" && issued != clientID {
		return errors.New("ChatGPT answered for a different app registration; start again")
	}

	tok, err := c.tokenRequest(ctx, url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {clientID},
		"code":          {code},
		"code_verifier": {att.verifier},
		"redirect_uri":  {RedirectURI},
		"resource":      {Resource},
	})
	if err != nil {
		return fmt.Errorf("ChatGPT sign-in failed: %w", err)
	}
	claims, err := c.validateIDToken(ctx, tok.IDToken, clientID, att.nonce)
	if err != nil {
		return fmt.Errorf("ChatGPT sign-in failed: %w", err)
	}
	reg := c.o.Secrets.ChatGPT()
	if reg.ClientID == clientID && reg.Subject != "" && reg.Subject != claims.Subject {
		return errors.New("that is a different ChatGPT account from the one this calendar was registered with")
	}
	scopes := strings.Fields(tok.Scope)
	if !slices.Contains(scopes, planScope) {
		return errors.New("ChatGPT plan use wasn't allowed; sign in again and allow it, or add an OpenAI API key instead")
	}
	now := c.o.Now()
	if err := c.o.Secrets.UpdateChatGPT(func(r *secrets.ChatGPT) {
		r.ClientID = clientID
		r.Subject = claims.Subject
		r.Email = claims.Email
		r.IDToken = tok.IDToken
		r.AccessToken = tok.AccessToken
		r.RefreshToken = tok.RefreshToken
		r.Scopes = scopes
		r.ExpiresAt = now.Add(time.Duration(tok.ExpiresIn) * time.Second)
	}); err != nil {
		return err
	}
	c.o.OnChange()
	return nil
}

// SignOut revokes the refresh token and forgets the tokens, keeping the
// issued client id and host id for the next sign-in. The tokens are always
// forgotten; a non-nil error means OpenAI did not confirm the revocation.
func (c *Client) SignOut(ctx context.Context) error {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	reg := c.o.Secrets.ChatGPT()
	var revokeErr error
	if reg.RefreshToken != "" {
		revokeErr = c.revoke(ctx, reg.ClientID, reg.RefreshToken)
	}
	if err := c.forgetTokens(); err != nil {
		return err
	}
	c.o.OnChange()
	if revokeErr != nil {
		return fmt.Errorf("chatgpt: remote revocation not confirmed: %w", revokeErr)
	}
	return nil
}

// AccessToken returns a current access token, refreshing it shortly before
// it expires. It returns "" when not signed in, and ErrSignedOut when a
// refresh shows the sign-in is no longer valid.
func (c *Client) AccessToken(ctx context.Context) (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	reg := c.o.Secrets.ChatGPT()
	if reg.AccessToken == "" {
		return "", nil
	}
	now := c.o.Now()
	if now.Before(reg.ExpiresAt.Add(-refreshMargin)) {
		return reg.AccessToken, nil
	}
	tok, err := c.tokenRequest(ctx, url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {reg.ClientID},
		"refresh_token": {reg.RefreshToken},
		"resource":      {Resource},
	})
	var oe *oauthError
	if errors.As(err, &oe) && terminalRefreshErrors[oe.Code] {
		if ferr := c.forgetTokens(); ferr != nil {
			return "", ferr
		}
		c.o.OnChange()
		return "", fmt.Errorf("%w: %v", ErrSignedOut, err)
	}
	if err != nil {
		return "", fmt.Errorf("chatgpt: refresh: %w", err)
	}
	idToken := reg.IDToken
	if tok.IDToken != "" {
		claims, err := c.validateIDToken(ctx, tok.IDToken, reg.ClientID, "")
		switch {
		case err != nil:
			log.Printf("chatgpt: ignoring refreshed ID token: %v", err)
		case claims.Subject != reg.Subject:
			log.Printf("chatgpt: ignoring refreshed ID token for a different account")
		default:
			idToken = tok.IDToken
		}
	}
	if err := c.o.Secrets.UpdateChatGPT(func(r *secrets.ChatGPT) {
		r.AccessToken = tok.AccessToken
		if tok.RefreshToken != "" {
			r.RefreshToken = tok.RefreshToken
		}
		if tok.Scope != "" {
			r.Scopes = strings.Fields(tok.Scope)
		}
		r.IDToken = idToken
		r.ExpiresAt = now.Add(time.Duration(tok.ExpiresIn) * time.Second)
	}); err != nil {
		return "", err
	}
	return tok.AccessToken, nil
}

func (c *Client) forgetTokens() error {
	return c.o.Secrets.UpdateChatGPT(func(r *secrets.ChatGPT) {
		r.IDToken = ""
		r.AccessToken = ""
		r.RefreshToken = ""
		r.Scopes = nil
		r.ExpiresAt = time.Time{}
	})
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
}

// oauthError is an error response from the token endpoint.
type oauthError struct {
	Status      int
	Code        string `json:"error"`
	Description string `json:"error_description"`
}

func (e *oauthError) Error() string {
	switch {
	case e.Description != "":
		return e.Description
	case e.Code != "":
		return e.Code
	}
	return fmt.Sprintf("token endpoint returned HTTP %d", e.Status)
}

func (c *Client) tokenRequest(ctx context.Context, form url.Values) (*tokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.o.Issuer+"/api/accounts/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := c.o.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		oe := &oauthError{Status: resp.StatusCode}
		_ = json.NewDecoder(resp.Body).Decode(oe)
		return nil, oe
	}
	var tok tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}
	if tok.AccessToken == "" {
		return nil, errors.New("token response has no access token")
	}
	return &tok, nil
}

// revoke ends the renewable session, retrying network failures and server
// errors with backoff. An HTTP 200 is success, even for an invalid token.
func (c *Client) revoke(ctx context.Context, clientID, refreshToken string) error {
	disc, err := c.discovery(ctx)
	if err != nil {
		return err
	}
	if disc.RevocationEndpoint == "" {
		return errors.New("no revocation endpoint published")
	}
	form := url.Values{"token": {refreshToken}, "token_type_hint": {"refresh_token"}, "client_id": {clientID}}
	backoff := 200 * time.Millisecond
	var lastErr error
	for i := range 3 {
		if i > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, disc.RevocationEndpoint, strings.NewReader(form.Encode()))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := c.o.HTTPClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return nil
		}
		lastErr = fmt.Errorf("revocation returned HTTP %d", resp.StatusCode)
		if resp.StatusCode < 500 {
			return lastErr
		}
	}
	return lastErr
}

func randomString() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

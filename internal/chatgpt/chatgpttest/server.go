// Package chatgpttest is an in-memory stand-in for OpenAI's "Sign in with
// ChatGPT" authorization server, for tests. It implements discovery, JWKS,
// the token endpoint (authorization_code with PKCE S256 and rotating
// refresh_token grants) and revocation, and plays the browser's part of the
// redirect.
package chatgpttest

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"
)

const (
	// IssuedClientID is the client id handed out on dynamic registration.
	IssuedClientID = "oaiapp_test123"
	// Email and Subject identify the signed-in test account.
	Email   = "parent@example.com"
	Subject = "user-sub-1"
	// PlanScope is the scope that authorizes ChatGPT plan usage.
	PlanScope = "chatgpt.tokens.use.direct"
	// FullScope is what a successful consent grants.
	FullScope = "chatgpt.tokens.use.direct email offline_access openid profile resource.invoke"
	keyID     = "test-key"
)

type pendingCode struct {
	clientID    string
	nonce       string
	challenge   string
	redirectURI string
	resource    string
}

// Server is a fake auth.openai.com. Exported fields may be changed between
// requests; they are read under the server's lock.
type Server struct {
	URL string

	key       *rsa.PrivateKey
	published *rsa.PublicKey
	srv       *httptest.Server

	mu sync.Mutex
	// Scope is the scope granted by code exchange (default FullScope).
	Scope string
	// Subject is the sub claim of issued ID tokens (default Subject).
	Subject string
	// ExpiresIn is the access-token lifetime in seconds (default 3600).
	ExpiresIn int
	// Now is the server's clock for token timestamps (default time.Now).
	Now func() time.Time
	// IDTokenClaims, when set, edits the ID token claims before signing.
	IDTokenClaims func(map[string]any)
	// IDTokenHeader, when set, edits the ID token's JOSE header before
	// signing; an "alg" of "none" produces an unsigned token.
	IDTokenHeader func(map[string]any)
	// RefreshIDToken makes refreshes also return a new ID token.
	RefreshIDToken bool
	// TokenError, when set, makes the next code exchanges fail with this
	// OAuth error code and description.
	TokenError, TokenErrorDescription string
	// RefreshError, when set, makes refreshes fail with this OAuth error.
	RefreshError string
	// RefreshStatus, when set, makes refreshes fail with this HTTP status
	// and no OAuth error body (a transient failure).
	RefreshStatus int
	// RefreshDelay holds each refresh open before answering.
	RefreshDelay time.Duration
	// RevokeStatus is the revocation endpoint's status (default 200).
	RevokeStatus int

	codes         map[string]pendingCode
	issued        int
	validRefresh  map[string]bool
	tokenRequests []url.Values
	revocations   []url.Values
	refreshes     int
	tokens        []string
}

func New(t *testing.T) *Server {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	s := &Server{
		key:          key,
		Scope:        FullScope,
		Subject:      Subject,
		ExpiresIn:    3600,
		Now:          time.Now,
		codes:        map[string]pendingCode{},
		validRefresh: map[string]bool{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", s.discovery)
	mux.HandleFunc("GET /.well-known/jwks.json", s.jwks)
	mux.HandleFunc("POST /api/accounts/oauth/token", s.token)
	mux.HandleFunc("POST /api/accounts/oauth/revoke", s.revoke)
	s.srv = httptest.NewServer(mux)
	s.URL = s.srv.URL
	t.Cleanup(s.srv.Close)
	return s
}

// Lock and Unlock guard changes to the exported knobs.
func (s *Server) Lock()   { s.mu.Lock() }
func (s *Server) Unlock() { s.mu.Unlock() }

// TokenRequests returns the form bodies posted to the token endpoint.
func (s *Server) TokenRequests() []url.Values {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]url.Values(nil), s.tokenRequests...)
}

// Revocations returns the form bodies posted to the revocation endpoint.
func (s *Server) Revocations() []url.Values {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]url.Values(nil), s.revocations...)
}

// IssuedTokens returns every access, refresh and ID token issued so far.
func (s *Server) IssuedTokens() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.tokens...)
}

// Refreshes counts refresh_token grants received.
func (s *Server) Refreshes() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.refreshes
}

// Callback plays the user approving authorizeURL in the browser and returns
// the URL the browser lands on. A dynamic-registration request gets the
// issued client id in the callback; a reauthorization does not.
func (s *Server) Callback(authorizeURL string) string {
	u, err := url.Parse(authorizeURL)
	if err != nil {
		panic(err)
	}
	q := u.Query()
	s.mu.Lock()
	defer s.mu.Unlock()
	clientID := q.Get("client_id")
	issue := clientID == "dynamic_agent_client"
	if issue {
		clientID = IssuedClientID
	}
	code := fmt.Sprintf("code-%d", len(s.codes)+1)
	s.codes[code] = pendingCode{
		clientID:    clientID,
		nonce:       q.Get("nonce"),
		challenge:   q.Get("code_challenge"),
		redirectURI: q.Get("redirect_uri"),
		resource:    q.Get("resource"),
	}
	cb := url.Values{"code": {code}, "state": {q.Get("state")}, "scope": {s.Scope}}
	if issue {
		cb.Set("client_id", IssuedClientID)
	}
	return q.Get("redirect_uri") + "?" + cb.Encode()
}

// CallbackWithClientID is Callback but the redirect names clientID.
func (s *Server) CallbackWithClientID(authorizeURL, clientID string) string {
	cb := s.Callback(authorizeURL)
	u, _ := url.Parse(cb)
	q := u.Query()
	q.Set("client_id", clientID)
	u.RawQuery = q.Encode()
	return u.String()
}

// CallbackWithoutClientID is Callback with the client_id parameter removed.
func (s *Server) CallbackWithoutClientID(authorizeURL string) string {
	cb := s.Callback(authorizeURL)
	u, _ := url.Parse(cb)
	q := u.Query()
	q.Del("client_id")
	u.RawQuery = q.Encode()
	return u.String()
}

// ErrorCallback plays the user declining (or the server failing) and returns
// the callback URL carrying an OAuth error.
func (s *Server) ErrorCallback(authorizeURL, code, description string) string {
	u, err := url.Parse(authorizeURL)
	if err != nil {
		panic(err)
	}
	q := u.Query()
	cb := url.Values{"error": {code}, "error_description": {description}, "state": {q.Get("state")}}
	return q.Get("redirect_uri") + "?" + cb.Encode()
}

func (s *Server) discovery(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                 s.URL,
		"authorization_endpoint": s.URL + "/api/accounts/authorize",
		"token_endpoint":         s.URL + "/api/accounts/oauth/token",
		"jwks_uri":               s.URL + "/.well-known/jwks.json",
		"revocation_endpoint":    s.URL + "/api/accounts/oauth/revoke",
	})
}

func (s *Server) jwks(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	pub := s.key.PublicKey
	if s.published != nil {
		pub = *s.published
	}
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"keys": []map[string]any{{
		"kty": "RSA",
		"kid": keyID,
		"use": "sig",
		"alg": "RS256",
		"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}}})
}

func oauthError(w http.ResponseWriter, status int, code, description string) {
	writeJSON(w, status, map[string]string{"error": code, "error_description": description})
}

func (s *Server) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_request", "body is not a form")
		return
	}
	form := r.PostForm
	s.mu.Lock()
	s.tokenRequests = append(s.tokenRequests, form)
	s.mu.Unlock()
	switch form.Get("grant_type") {
	case "authorization_code":
		s.exchange(w, form)
	case "refresh_token":
		s.refresh(w, form)
	default:
		oauthError(w, http.StatusBadRequest, "unsupported_grant_type", "unsupported grant")
	}
}

func (s *Server) exchange(w http.ResponseWriter, form url.Values) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.TokenError != "" {
		oauthError(w, http.StatusBadRequest, s.TokenError, s.TokenErrorDescription)
		return
	}
	p, ok := s.codes[form.Get("code")]
	delete(s.codes, form.Get("code"))
	sum := sha256.Sum256([]byte(form.Get("code_verifier")))
	switch {
	case !ok:
		oauthError(w, http.StatusBadRequest, "invalid_grant", "Unknown or reused authorization code.")
	case form.Get("client_id") != p.clientID:
		oauthError(w, http.StatusBadRequest, "invalid_grant", "client_id does not match the authorization request.")
	case base64.RawURLEncoding.EncodeToString(sum[:]) != p.challenge:
		oauthError(w, http.StatusBadRequest, "invalid_grant", "PKCE verification failed.")
	case form.Get("redirect_uri") != p.redirectURI:
		oauthError(w, http.StatusBadRequest, "invalid_grant", "redirect_uri does not match.")
	case form.Get("resource") != p.resource:
		oauthError(w, http.StatusBadRequest, "invalid_target", "resource does not match.")
	default:
		s.issueLocked(w, p.clientID, p.nonce, true)
	}
}

func (s *Server) refresh(w http.ResponseWriter, form url.Values) {
	s.mu.Lock()
	s.refreshes++
	delay := s.RefreshDelay
	s.mu.Unlock()
	time.Sleep(delay)
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case s.RefreshStatus != 0:
		w.WriteHeader(s.RefreshStatus)
	case s.RefreshError != "":
		oauthError(w, http.StatusBadRequest, s.RefreshError, "The refresh token is no longer valid.")
	case !s.validRefresh[form.Get("refresh_token")]:
		oauthError(w, http.StatusBadRequest, "refresh_token_reused", "The refresh token was already used.")
	case form.Get("client_id") != IssuedClientID:
		oauthError(w, http.StatusUnauthorized, "invalid_client", "Unknown client.")
	case form.Get("resource") != "https://api.openai.com/v1":
		oauthError(w, http.StatusBadRequest, "invalid_target", "resource does not match.")
	default:
		delete(s.validRefresh, form.Get("refresh_token"))
		s.issueLocked(w, form.Get("client_id"), "", s.RefreshIDToken)
	}
}

func (s *Server) issueLocked(w http.ResponseWriter, clientID, nonce string, withIDToken bool) {
	s.issued++
	access := fmt.Sprintf("at-%d", s.issued)
	refresh := fmt.Sprintf("rt-%d", s.issued)
	s.validRefresh[refresh] = true
	now := s.Now()
	body := map[string]any{
		"access_token":        access,
		"refresh_token":       refresh,
		"token_type":          "Bearer",
		"expires_in":          s.ExpiresIn,
		"scope":               s.Scope,
		"earliest_refresh_at": now.Unix(),
	}
	if withIDToken {
		claims := map[string]any{
			"iss":   s.URL,
			"aud":   clientID,
			"sub":   s.Subject,
			"email": Email,
			"iat":   now.Unix(),
			"exp":   now.Add(time.Hour).Unix(),
		}
		if nonce != "" {
			claims["nonce"] = nonce
		}
		if s.IDTokenClaims != nil {
			s.IDTokenClaims(claims)
		}
		body["id_token"] = s.sign(claims)
		s.tokens = append(s.tokens, body["id_token"].(string))
	}
	s.tokens = append(s.tokens, access, refresh)
	writeJSON(w, http.StatusOK, body)
}

// sign returns an RS256 JWT of claims signed with the server's key.
func (s *Server) sign(claims map[string]any) string {
	h := map[string]any{"alg": "RS256", "typ": "JWT", "kid": keyID}
	if s.IDTokenHeader != nil {
		s.IDTokenHeader(h)
	}
	header, _ := json.Marshal(h)
	payload, _ := json.Marshal(claims)
	input := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	if h["alg"] == "none" {
		return input + "."
	}
	sum := sha256.Sum256([]byte(input))
	sig, err := rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, sum[:])
	if err != nil {
		panic(err)
	}
	return input + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// ForgeIDTokenSignature makes subsequent ID tokens carry a signature from a
// key other than the one published in the JWKS.
func (s *Server) ForgeIDTokenSignature(t *testing.T) {
	t.Helper()
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	pub := s.key.PublicKey
	s.published = &pub
	s.key = other
}

func (s *Server) revoke(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	s.mu.Lock()
	s.revocations = append(s.revocations, r.PostForm)
	delete(s.validRefresh, r.PostForm.Get("token"))
	status := s.RevokeStatus
	s.mu.Unlock()
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

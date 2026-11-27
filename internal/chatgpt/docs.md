# Noridoc: chatgpt

Path: @/internal/chatgpt

### Overview

- Implements OpenAI's official "Sign in with ChatGPT" (SIWC) flow for open-source, locally hosted apps (developers.openai.com/siwc), so Ziggy can run on the signed-in user's ChatGPT plan instead of a paid API key.
- It is its own SIWC client registration (`AppName` "Home Calendar"), not a reuse of the Codex CLI client id, whose terms forbid impersonating it.
- The resulting access token is used as the bearer credential for the Responses API by @/internal/agent. The `chatgpttest` subpackage is an in-memory fake authorization server for tests.

### How it fits into the larger codebase

```
 admin (browser)                         internal/server agent_auth.go
   "Sign in with ChatGPT" ---POST start---> Client.Start() --> authorizeUrl
   opens authorizeUrl in a tab
   user signs in at auth.openai.com
   tab lands on http://127.0.0.1:1455/auth/callback?code=...&state=...  (nothing listens)
   user pastes that URL ---POST complete--> Client.Complete(redirectURL)
                                              |-- token exchange, ID token validation
                                              `-- secrets.UpdateChatGPT  -> OnChange
                                                                            = Server.refreshAgent
 Agent turn --> Server.agentToken --> Client.AccessToken (refresh if near expiry)
 admin "Sign out" ---DELETE chatgpt---> Client.SignOut (revoke + forget tokens)
```

- Constructed by `Server.newAgent` in @/internal/server/agent.go with the secrets store and `OnChange: s.refreshAgent`, so the agent's readiness and the admin status update whenever sign-in state changes (including when a refresh discovers the sign-in was revoked).
- All persistent state lives in @/internal/secrets (`secrets.ChatGPT`). The only in-memory state is pending sign-in attempts.
- `Server.agentToken` maps `ErrSignedOut` to `agent.ErrSignInAgain`, which @/internal/agent turns into a spoken "sign in again" reply.

### Core Implementation

- **Paste-back redirect**: `RedirectURI` is the loopback `http://127.0.0.1:1455/auth/callback`, and no server listens there. The browser lands on an unloadable page and the user copies its address into admin. This works from any device (wall tablet, phone) because the code exchange happens on the server, not on the device; SIWC has no device-code flow.
- **`Start`**: generates `state`, `nonce` and a PKCE verifier (S256 challenge), and builds `/api/accounts/authorize` with `Scope`, `Resource`, and the stored `ext_agent_host_id`. On the first sign-in (no stored client id) it uses `client_id=dynamic_agent_client` plus `agent_name_hint`, which makes OpenAI register a per-user client; afterwards it uses the issued `oaiapp_...` client id with `id_token_hint` / `login_hint`. The attempt (client id, verifier, nonce, start time) is kept in memory for `attemptTTL` (10 minutes) keyed by `state`; expired attempts are pruned on each `Start`.
- **`Complete`**: parses the pasted URL, consumes the attempt by `state` (single use), surfaces `error` / `error_description` from the callback, and takes the issued client id from the callback's `client_id` for dynamic registration (rejecting a missing one, or a mismatch on a reused registration). It exchanges the code (`authorization_code` + `code_verifier`), validates the ID token, rejects a different account than the one already registered (`Subject` mismatch with the same client id), requires the granted scopes to include `chatgpt.tokens.use.direct` (plan usage), then saves client id, identity, tokens, scopes and `ExpiresAt`.
- **ID token validation (`idtoken.go`)**: stdlib-only RS256. Fetches OpenID discovery and the JWKS on each validation, picks the RSA key by `kid`, verifies the PKCS#1 v1.5 signature, and checks `alg == RS256`, issuer equals the configured issuer, audience contains the client id, non-empty `sub`, not expired, and nonce (when one is expected; refresh validation passes no nonce).
- **`AccessToken`**: returns `""` when not signed in, the stored token when it is more than `refreshMargin` (5 minutes) from expiry, otherwise refreshes. `tokenMu` serializes refreshes and sign-out, so concurrent turns share one refresh and a rotating refresh token is never used twice. A rotated refresh token and new scopes are persisted. A refreshed ID token is only kept if it validates and has the same subject; otherwise it is logged and the old one kept.
- **Terminal refresh errors** (`invalid_grant`, `refresh_token_expired`, `refresh_token_reused`, ... in `terminalRefreshErrors`) forget the tokens, fire `OnChange`, and return `ErrSignedOut`. Other refresh failures (network, 5xx) return a plain error and keep the sign-in.
- **`SignOut`**: revokes the refresh token at the discovery document's `revocation_endpoint` (up to three attempts with doubling backoff for network errors and 5xx; any 4xx stops; HTTP 200 is success). Tokens are always forgotten regardless of the revocation result; client id, host id and subject are kept. A revocation failure is returned as an error after the tokens are cleared, which the server turns into a `warning` in a 200 response.

### Things to Know

- `Status().SignedIn` is simply "an access token is stored"; it does not check expiry. Expiry is handled lazily by `AccessToken`.
- Error strings from `Complete` are user-facing (the server returns them verbatim as `{"error"}` for 400s), which is why they are written as plain instructions.
- `Options.Issuer` (and `server.Options.ChatGPTIssuer`) exists so tests point the client at `chatgpttest.Server`, which implements discovery, JWKS, both token grants with PKCE checks and rotating refresh tokens, revocation, and helpers that play the browser's redirect (`Callback`, `ErrorCallback`, variants with/without a client id) and forge bad signatures.

Created and maintained by Nori.

# Noridoc: secrets

Path: @/internal/secrets

### Overview

- Owns `secrets.json`, the owner-only file that holds the assistant's credentials: the OpenAI API key, the Google service-account JSON key, and the "Sign in with ChatGPT" registration (issued client id, host id, account identity and tokens).
- Exists separately from @/internal/config because `config.json` is served to every browser without authentication (`GET /api/config`, `config` WebSocket frames). Nothing in this store is ever part of that config.
- Environment variables `OPENAI_API_KEY` and `GOOGLE_SERVICE_ACCOUNT_JSON` override the stored values; they are optional overrides, not the primary way to configure Ziggy.

### How it fits into the larger codebase

```
 cmd/server openStores(CONFIG_PATH)
   |-- config.Open(config.json)
   `-- secrets.Open(PathFor(config.json))  -> secrets.json beside it
            |
            v
 internal/server  (Options.Secrets)
   |-- agent_auth.go   admin credential endpoints: Set*/Clear* , status flags only
   |-- agent.go        refreshAgent / agentToken / googleCalendarClient read values
   `-- internal/chatgpt Client   ChatGPT() / UpdateChatGPT() for sign-in state
```

- Opened once at startup by `openStores` in @/cmd/server/main.go, at `PathFor(configPath)`, i.e. `secrets.json` in the same directory as `config.json` (in Docker, `/data/secrets.json` on the bind-mounted volume).
- @/internal/server reads values to build the agent's credentials and the Google Calendar client, and writes them from the admin endpoints. HTTP responses only ever expose whether a value is set and whether it came from the environment (`Value.FromEnv`), never the value.
- @/internal/chatgpt is the only reader/writer of the `ChatGPT` struct.

### Core Implementation

- `Open(path)`: a missing file is fine; an unreadable or unparseable file is an error that fails server startup. It is deliberately not recreated, because doing so would lose the host id and the ChatGPT refresh token. If no `ChatGPT.HostID` is present, one is generated (`urn:uuid:<uuid>`) and saved immediately, so the host id is stable for the lifetime of the file.
- Environment overrides are read once, in `Open`. When an override is set, the getter returns it with `FromEnv: true`, and `SetOpenAIKey` / `SetGoogleServiceAccount` (and the `Clear*` wrappers, which set `""`) return `ErrFromEnv` without touching the file. The server maps `ErrFromEnv` to HTTP 409.
- Writes go through `update`, which applies the change to a copy, saves it, and only then swaps it into memory, so a failed write leaves the in-memory state unchanged. `save` writes a `.secrets-*.tmp` file in the same directory, `chmod 0600`, `fsync`, then renames it over `secrets.json`, so a crash never leaves a partial file.
- `ChatGPT()` returns a copy (with `Scopes` cloned); `UpdateChatGPT(fn)` mutates the registration under the same mutex and persists atomically.

| Stored field | Meaning |
|---|---|
| `openaiApiKey` | Fallback OpenAI API key |
| `googleServiceAccount` | Google service-account JSON key (or, via env, a path) |
| `chatgpt.clientId` | Per-user `oaiapp_...` client id issued on first ChatGPT sign-in; empty before then |
| `chatgpt.hostId` | Stable `ext_agent_host_id` for this install |
| `chatgpt.subject` / `email` | Account identity from the validated ID token |
| `chatgpt.idToken` / `accessToken` / `refreshToken` / `scopes` / `expiresAt` | Current sign-in; cleared on sign-out |

### Things to Know

- Signing out of ChatGPT clears tokens but keeps `clientId`, `hostId` and `subject`, so the next sign-in reuses the same registration and is checked against the same account (see @/internal/chatgpt).
- `secrets.json` must be backed up with `config.json`; losing it means signing in to ChatGPT again (as a new registration) and re-entering the key and service account.
- The env value for the Google service account may be inline JSON or a file path; interpretation happens later in @/internal/gcal `LoadCredentials`, not here.

Created and maintained by Nori.

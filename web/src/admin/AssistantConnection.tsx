import { useState, type ReactNode } from "react";
import type { AgentAuth } from "../types";
import {
  clearAgentApiKey,
  clearAgentGoogle,
  clearAgentHomeAssistant,
  completeChatGPTSignIn,
  putAgentApiKey,
  putAgentGoogle,
  putAgentHomeAssistant,
  signOutChatGPT,
  startChatGPTSignIn,
} from "../api";

const SETUP_URL = "https://github.com/jpatters/home-calendar#assistant-setup-ziggy";

const SUMMARY: Record<AgentAuth["using"], string> = {
  chatgpt: "Using: ChatGPT subscription",
  apiKey: "Using: API key",
  none: "Not connected",
};

interface Props {
  auth: AgentAuth;
  onAuth: (a: AgentAuth) => void;
}

// Runs one request at a time and keeps the server's error message for display.
function useAction() {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const run = async <T,>(request: () => Promise<T>): Promise<T | undefined> => {
    setBusy(true);
    setError(null);
    try {
      return await request();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      return undefined;
    } finally {
      setBusy(false);
    }
  };
  return { busy, error, run };
}

// Credentials are sent to their own endpoints as soon as they are saved and
// are never read back; the server only reports whether each one is present.
export default function AssistantConnection({ auth, onAuth }: Props) {
  return (
    <div className="assistant-connection">
      <h3>Connection</h3>
      <div className="hint">
        <span>{SUMMARY[auth.using]}</span> ·{" "}
        <a href={SETUP_URL} target="_blank" rel="noopener noreferrer">Setup instructions</a>
      </div>
      <ChatGPTAccount auth={auth} onAuth={onAuth} />
      <ApiKey auth={auth} onAuth={onAuth} />
      <GoogleCalendar auth={auth} onAuth={onAuth} />
      <HomeAssistant auth={auth} onAuth={onAuth} />
    </div>
  );
}

function ChatGPTAccount({ auth, onAuth }: Props) {
  const { busy, error, run } = useAction();
  const [authorizeUrl, setAuthorizeUrl] = useState<string | null>(null);
  const [redirectUrl, setRedirectUrl] = useState("");
  const [warning, setWarning] = useState("");

  const start = async () => {
    setWarning("");
    const res = await run(startChatGPTSignIn);
    if (!res) return;
    setAuthorizeUrl(res.authorizeUrl);
    window.open(res.authorizeUrl, "_blank", "noopener");
  };

  const finish = async () => {
    const next = await run(() => completeChatGPTSignIn(redirectUrl));
    if (!next) return;
    setAuthorizeUrl(null);
    setRedirectUrl("");
    onAuth(next);
  };

  const signOut = async () => {
    const next = await run(signOutChatGPT);
    if (!next) return;
    setWarning(next.warning ?? "");
    onAuth(next);
  };

  return (
    <fieldset>
      <legend>ChatGPT account</legend>
      {auth.chatgpt.signedIn ? (
        <div className="credential-row">
          <span>{`Signed in as ${auth.chatgpt.email}`}</span>
          <button disabled={busy} onClick={() => void signOut()}>Sign out</button>
        </div>
      ) : (
        <>
          <button disabled={busy} onClick={() => void start()}>Sign in with ChatGPT</button>
          {authorizeUrl && (
            <>
              <div className="hint">
                If no tab opened, <a href={authorizeUrl} target="_blank" rel="noopener noreferrer">{authorizeUrl}</a>
              </div>
              <label>
                <span>Paste the address from the sign-in tab</span>
                <input
                  type="text"
                  autoComplete="off"
                  value={redirectUrl}
                  onChange={(e) => setRedirectUrl(e.target.value)}
                />
              </label>
              <div className="hint">
                After you sign in, that tab shows a page that can't load. Copy its whole address and paste it here.
              </div>
              <button disabled={busy || !redirectUrl.trim()} onClick={() => void finish()}>Finish sign-in</button>
            </>
          )}
        </>
      )}
      {warning && <div className="error">{warning}</div>}
      {error && <div className="error">{error}</div>}
    </fieldset>
  );
}

interface SecretProps extends Props {
  legend: string;
  label: string;
  envVar: string;
  state: { set: boolean; fromEnv: boolean };
  multiline?: boolean;
  save: (secret: string) => Promise<AgentAuth>;
  clear: () => Promise<AgentAuth>;
  savedNote?: string;
  help?: ReactNode;
}

function SecretField({ onAuth, legend, label, envVar, state, multiline, save, clear, savedNote, help }: SecretProps) {
  const { busy, error, run } = useAction();
  const [secret, setSecret] = useState("");
  const locked = state.fromEnv;

  const onSave = async () => {
    const next = await run(() => save(secret));
    if (!next) return;
    setSecret("");
    onAuth(next);
  };

  const onClear = async () => {
    const next = await run(clear);
    if (next) onAuth(next);
  };

  const status = locked ? `Set by environment (${envVar})` : state.set ? "Saved" : "Not set";

  return (
    <fieldset>
      <legend>{legend}</legend>
      {help}
      <label>
        <span>{label}</span>
        {multiline ? (
          <textarea
            rows={4}
            autoComplete="off"
            spellCheck={false}
            disabled={locked}
            value={secret}
            onChange={(e) => setSecret(e.target.value)}
          />
        ) : (
          <input
            type="password"
            autoComplete="off"
            disabled={locked}
            value={secret}
            onChange={(e) => setSecret(e.target.value)}
          />
        )}
      </label>
      <div className="credential-row">
        <span className="hint">{status}</span>
        <button disabled={locked || busy} onClick={() => void onSave()}>Save</button>
        <button disabled={locked || busy} onClick={() => void onClear()}>Clear</button>
      </div>
      {state.set && savedNote && <div className="hint">{savedNote}</div>}
      {error && <div className="error">{error}</div>}
    </fieldset>
  );
}

function ApiKey({ auth, onAuth }: Props) {
  return (
    <SecretField
      auth={auth}
      onAuth={onAuth}
      legend="OpenAI API key"
      label="API key (optional fallback)"
      envVar="OPENAI_API_KEY"
      state={auth.apiKey}
      save={putAgentApiKey}
      clear={clearAgentApiKey}
    />
  );
}

function GoogleCalendar({ auth, onAuth }: Props) {
  return (
    <SecretField
      auth={auth}
      onAuth={onAuth}
      legend="Google Calendar"
      label="Service account JSON key"
      envVar="GOOGLE_SERVICE_ACCOUNT_JSON"
      state={auth.google}
      multiline
      save={putAgentGoogle}
      clear={clearAgentGoogle}
      savedNote={`Share each calendar with ${auth.google.serviceAccountEmail} (Make changes to events)`}
      help={<GoogleKeySteps />}
    />
  );
}

function ConsoleLink({ href, children }: { href: string; children: ReactNode }) {
  return (
    <a href={href} target="_blank" rel="noopener noreferrer">
      {children}
    </a>
  );
}

// Step-by-step setup for the Google service-account key that lets Ziggy change
// calendar events; reading calendars works without it.
function GoogleKeySteps() {
  return (
    <details className="setup-steps">
      <summary>How to get a service account key</summary>
      <ol>
        <li>
          In the Google Cloud console,{" "}
          <ConsoleLink href="https://console.cloud.google.com/projectcreate">create a project</ConsoleLink> (any
          name, such as "Home calendar"), or pick an existing one.
        </li>
        <li>
          <ConsoleLink href="https://console.cloud.google.com/apis/library/calendar-json.googleapis.com">
            Enable the Google Calendar API
          </ConsoleLink>{" "}
          for that project.
        </li>
        <li>
          Open{" "}
          <ConsoleLink href="https://console.cloud.google.com/iam-admin/serviceaccounts">Service accounts</ConsoleLink>{" "}
          and choose <strong>Create service account</strong>. Give it a name such as "ziggy", then press{" "}
          <strong>Done</strong>; it needs no roles or permissions.
        </li>
        <li>
          Open the new service account, go to the <strong>Keys</strong> tab, and choose{" "}
          <strong>Add key → Create new key → JSON</strong>. Your browser downloads a <code>.json</code> file.
        </li>
        <li>
          Open that file in a text editor, copy everything in it, paste it below, and press <strong>Save</strong>.
          The service account's email address then appears here.
        </li>
        <li>
          In{" "}
          <ConsoleLink href="https://calendar.google.com/calendar/r/settings">Google Calendar settings</ConsoleLink>,
          open each calendar Ziggy should be able to change, go to{" "}
          <strong>Share with specific people or groups → Add people and groups</strong>, enter that email address,
          and choose <strong>Make changes to events</strong>.
        </li>
      </ol>
      <p className="hint">
        Only calendars added in admin with their Google "Secret address in iCal format" can be changed. Keep the
        downloaded file private; it grants access to every calendar you share with it.
      </p>
    </details>
  );
}

// The Home Assistant connection takes an address and a token together; the
// server checks them against Home Assistant before saving.
function HomeAssistant({ auth, onAuth }: Props) {
  const { busy, error, run } = useAction();
  const [url, setUrl] = useState("");
  const [token, setToken] = useState("");
  const ha = auth.homeAssistant;
  const locked = ha.fromEnv;

  const connect = async () => {
    const next = await run(() => putAgentHomeAssistant(url, token));
    if (!next) return;
    setUrl("");
    setToken("");
    onAuth(next);
  };

  const disconnect = async () => {
    const next = await run(clearAgentHomeAssistant);
    if (next) onAuth(next);
  };

  const status = locked
    ? `Set by environment (HOME_ASSISTANT_TOKEN): ${ha.url}`
    : ha.set
      ? `Connected to ${ha.url}`
      : "Not set up";

  return (
    <fieldset>
      <legend>Home Assistant</legend>
      <HomeAssistantSteps />
      <label>
        <span>Address</span>
        <input
          type="url"
          autoComplete="off"
          placeholder="http://homeassistant.local:8123"
          disabled={locked}
          value={url}
          onChange={(e) => setUrl(e.target.value)}
        />
      </label>
      <label>
        <span>Long-lived access token</span>
        <input
          type="password"
          autoComplete="off"
          disabled={locked}
          value={token}
          onChange={(e) => setToken(e.target.value)}
        />
      </label>
      <div className="credential-row">
        <span className="hint">{status}</span>
        <button disabled={locked || busy || !url.trim() || !token.trim()} onClick={() => void connect()}>
          Connect
        </button>
        <button disabled={locked || busy || !ha.set} onClick={() => void disconnect()}>
          Disconnect
        </button>
      </div>
      {error && <div className="error">{error}</div>}
    </fieldset>
  );
}

function HomeAssistantSteps() {
  return (
    <details className="setup-steps">
      <summary>How to connect Home Assistant</summary>
      <ol>
        <li>
          Optional but safer: in Home Assistant, go to <strong>Settings → People → Users</strong> and add a user
          such as "Ziggy" that is <strong>not an administrator</strong>, then log in as that user for the next step.
        </li>
        <li>
          Open your <strong>Profile → Security</strong>, and under <strong>Long-lived access tokens</strong> choose{" "}
          <strong>Create token</strong>. Copy it right away; it's only shown once.
        </li>
        <li>
          Enter your Home Assistant's address (the one you open in a browser, such as http://192.168.1.20:8123) and
          the token below, then press <strong>Connect</strong>.
        </li>
        <li>
          Ziggy can only use devices exposed to Assist: <strong>Settings → Voice assistants → Expose</strong>.
          Locks, alarms, garage doors and gates are always look-only.
        </li>
      </ol>
    </details>
  );
}

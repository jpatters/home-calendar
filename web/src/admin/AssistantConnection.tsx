import { useState, type ReactNode } from "react";
import type { AgentAuth } from "../types";
import {
  clearAgentApiKey,
  clearAgentGoogle,
  completeChatGPTSignIn,
  putAgentApiKey,
  putAgentGoogle,
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
      <div className="hint">
        Ziggy uses your ChatGPT plan for everyone who talks to it. OpenAI's Sign in with ChatGPT terms expect only
        the signed-in person's own activity to use their plan, so use an API key if others in the house will talk to
        Ziggy.
      </div>
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

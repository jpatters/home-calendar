import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import AssistantPanel from "./AssistantPanel";
import Admin from "./Admin";
import { buildConfig, buildLive } from "../display/displayFixtures";
import type { Agent, AgentAuth, Config } from "../types";

function agentConfig(overrides: Partial<Agent> = {}): Agent {
  return {
    enabled: false,
    model: "gpt-6.1-terra",
    effort: "low",
    textInputEnabled: false,
    quietStart: "22:00",
    quietEnd: "07:00",
    voiceDelayMs: 0,
    ...overrides,
  };
}

interface AuthOverrides {
  using?: AgentAuth["using"];
  chatgpt?: Partial<AgentAuth["chatgpt"]>;
  apiKey?: Partial<AgentAuth["apiKey"]>;
  google?: Partial<AgentAuth["google"]>;
}

function agentAuth(overrides: AuthOverrides = {}): AgentAuth {
  return {
    using: overrides.using ?? "none",
    chatgpt: { signedIn: false, email: "", ...overrides.chatgpt },
    apiKey: { set: false, fromEnv: false, ...overrides.apiKey },
    google: { set: false, fromEnv: false, serviceAccountEmail: "", ...overrides.google },
  };
}

const AUTHORIZE_URL = "https://auth.openai.com/oauth/authorize?client_id=abc&state=xyz";
const SERVICE_ACCOUNT_EMAIL = "ziggy@home-calendar.iam.gserviceaccount.com";

type Handler = (init?: RequestInit) => Response | Promise<Response>;

let auth: AgentAuth = agentAuth();
let routes: Record<string, Handler> = {};
const fetchMock = vi.fn();

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status });
}

function bodyOf(init?: RequestInit): unknown {
  return JSON.parse(String(init?.body));
}

function pathOf(url: unknown): string {
  return new URL(String(url), "http://localhost").pathname;
}

// The server's credential endpoints, backed by `auth`; tests replace entries
// in `routes` to simulate rejections. PUT /api/config echoes the body.
// Anything else 404s.
function defaultRoutes(): Record<string, Handler> {
  const using = (a: AgentAuth): AgentAuth["using"] =>
    a.chatgpt.signedIn ? "chatgpt" : a.apiKey.set ? "apiKey" : "none";
  const update = (next: AuthOverrides) => {
    const merged = agentAuth({ ...auth, ...next });
    auth = { ...merged, using: using(merged) };
    return json(auth);
  };
  return {
    "GET /api/agent/auth": () => json(auth),
    "POST /api/agent/auth/chatgpt/start": () => json({ authorizeUrl: AUTHORIZE_URL }),
    "POST /api/agent/auth/chatgpt/complete": () => update({ chatgpt: { signedIn: true, email: "jordan@example.com" } }),
    "DELETE /api/agent/auth/chatgpt": () => update({ chatgpt: { signedIn: false, email: "" } }),
    "PUT /api/agent/auth/api-key": () => update({ apiKey: { set: true, fromEnv: false } }),
    "DELETE /api/agent/auth/api-key": () => update({ apiKey: { set: false, fromEnv: false } }),
    "PUT /api/agent/auth/google": () =>
      update({ google: { set: true, fromEnv: false, serviceAccountEmail: SERVICE_ACCOUNT_EMAIL } }),
    "DELETE /api/agent/auth/google": () => update({ google: { set: false, fromEnv: false, serviceAccountEmail: "" } }),
    "PUT /api/config": (init) => new Response(String(init?.body), { status: 200 }),
  };
}

beforeEach(() => {
  auth = agentAuth();
  routes = defaultRoutes();
  fetchMock.mockImplementation(async (url: string, init?: RequestInit) => {
    const handler = routes[`${init?.method ?? "GET"} ${pathOf(url)}`];
    return handler ? handler(init) : new Response("not found", { status: 404 });
  });
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  vi.clearAllMocks();
});

function requests(method: string, path: string) {
  return fetchMock.mock.calls.filter(([url, init]) => (init?.method ?? "GET") === method && pathOf(url) === path);
}

function group(name: string) {
  return within(screen.getByRole("group", { name }));
}

// Holds a route's response until release() is called, so a test can act while
// the request is in flight.
function holdRoute(key: string) {
  const original = routes[key];
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  routes[key] = async (init) => {
    await gate;
    return original(init);
  };
  return () => act(async () => release());
}

function renderPanel(value: Agent = agentConfig()) {
  let captured: Agent | null = null;
  render(<AssistantPanel value={value} onChange={(a) => { captured = a; }} />);
  return () => captured;
}

describe("AssistantPanel", () => {
  test.each<[string, () => void, Partial<Agent>]>([
    ["the enable toggle", () => fireEvent.click(screen.getByRole("checkbox", { name: /enable ziggy/i })), { enabled: true }],
    [
      "the text box toggle",
      () => fireEvent.click(screen.getByRole("checkbox", { name: /let people type to ziggy on the display/i })),
      { textInputEnabled: true },
    ],
    [
      "the model",
      () => fireEvent.change(screen.getByRole("textbox", { name: /model/i }), { target: { value: "gpt-7" } }),
      { model: "gpt-7" },
    ],
    [
      "the reasoning effort",
      () => fireEvent.change(screen.getByRole("combobox", { name: /reasoning effort/i }), { target: { value: "high" } }),
      { effort: "high" },
    ],
    [
      "quiet hours start",
      () => fireEvent.change(screen.getByLabelText(/quiet hours start/i), { target: { value: "21:30" } }),
      { quietStart: "21:30" },
    ],
    [
      "quiet hours end",
      () => fireEvent.change(screen.getByLabelText(/quiet hours end/i), { target: { value: "06:45" } }),
      { quietEnd: "06:45" },
    ],
    [
      "the voice delay in seconds",
      () => fireEvent.change(screen.getByLabelText(/voice delay/i), { target: { value: "1.5" } }),
      { voiceDelayMs: 1500 },
    ],
  ])("editing %s propagates", (_name, edit, expected) => {
    const initial = agentConfig();
    const changed = renderPanel(initial);
    edit();
    expect(changed()).toEqual({ ...initial, ...expected });
  });

  test("shows the saved voice delay in seconds", () => {
    renderPanel(agentConfig({ voiceDelayMs: 1250 }));
    expect((screen.getByLabelText(/voice delay/i) as HTMLInputElement).value).toBe("1.25");
  });

  test("offers every reasoning effort", () => {
    renderPanel();
    const select = screen.getByRole("combobox", { name: /reasoning effort/i }) as HTMLSelectElement;
    expect(Array.from(select.options).map((o) => o.value)).toEqual([
      "none",
      "minimal",
      "low",
      "medium",
      "high",
      "xhigh",
      "max",
    ]);
  });

  test("shows the saved settings", () => {
    renderPanel(agentConfig({ model: "gpt-x", effort: "medium", quietStart: "23:00", quietEnd: "08:00" }));
    expect((screen.getByRole("textbox", { name: /model/i }) as HTMLInputElement).value).toBe("gpt-x");
    expect((screen.getByRole("combobox", { name: /reasoning effort/i }) as HTMLSelectElement).value).toBe("medium");
    expect((screen.getByLabelText(/quiet hours start/i) as HTMLInputElement).value).toBe("23:00");
    expect((screen.getByLabelText(/quiet hours end/i) as HTMLInputElement).value).toBe("08:00");
  });

});

describe("AssistantPanel connection", () => {
  test.each<[AgentAuth["using"], string]>([
    ["chatgpt", "Using: ChatGPT subscription"],
    ["apiKey", "Using: API key"],
    ["none", "Not connected"],
  ])("summarizes the connection in use (%s)", async (using, summary) => {
    auth = agentAuth({ using });
    renderPanel();
    expect(await screen.findByText(summary)).toBeTruthy();
  });

  test("says the status is unavailable when it cannot be fetched", async () => {
    routes["GET /api/agent/auth"] = () => json({ error: "boom" }, 500);
    renderPanel();
    expect(await screen.findByText("Status unavailable")).toBeTruthy();
    expect(screen.queryByText(/using:/i)).toBeNull();
    expect(screen.queryByText("Not connected")).toBeNull();
    expect(screen.queryByRole("group", { name: "ChatGPT account" })).toBeNull();
    expect(screen.queryByRole("group", { name: "OpenAI API key" })).toBeNull();
    expect(screen.queryByRole("group", { name: "Google Calendar" })).toBeNull();
  });

  test("links to the setup instructions in a new tab", async () => {
    renderPanel();
    const link = (await screen.findByRole("link", { name: "Setup instructions" })) as HTMLAnchorElement;
    expect(link.getAttribute("href")).toBe("https://github.com/jpatters/home-calendar#assistant-setup-ziggy");
    expect(link.getAttribute("target")).toBe("_blank");
    expect(link.getAttribute("rel")).toMatch(/noopener/);
  });

  describe("ChatGPT account", () => {
    test("starting sign-in opens the authorize URL in a new tab and offers it as a link", async () => {
      const open = vi.spyOn(window, "open").mockReturnValue(null);
      renderPanel();
      const chatgpt = await waitFor(() => group("ChatGPT account"));
      fireEvent.click(chatgpt.getByRole("button", { name: "Sign in with ChatGPT" }));

      await waitFor(() => expect(open).toHaveBeenCalled());
      expect(requests("POST", "/api/agent/auth/chatgpt/start")).toHaveLength(1);
      expect(open.mock.calls[0][0]).toBe(AUTHORIZE_URL);
      const fallback = chatgpt.getAllByRole("link").find((l) => l.getAttribute("href") === AUTHORIZE_URL);
      expect(fallback).toBeTruthy();
      expect(chatgpt.getByLabelText("Paste the address from the sign-in tab")).toBeTruthy();
      expect(chatgpt.getByText(/paste it here/i)).toBeTruthy();
    });

    test("finishing sign-in sends the pasted address and shows the signed-in account", async () => {
      vi.spyOn(window, "open").mockReturnValue(null);
      renderPanel();
      const chatgpt = await waitFor(() => group("ChatGPT account"));
      fireEvent.click(chatgpt.getByRole("button", { name: "Sign in with ChatGPT" }));
      const field = await chatgpt.findByLabelText("Paste the address from the sign-in tab");
      const redirect = "http://localhost:1455/auth/callback?code=c0de&state=xyz";
      fireEvent.change(field, { target: { value: redirect } });
      fireEvent.click(chatgpt.getByRole("button", { name: "Finish sign-in" }));

      expect(await chatgpt.findByText("Signed in as jordan@example.com")).toBeTruthy();
      const [, init] = requests("POST", "/api/agent/auth/chatgpt/complete")[0];
      expect(bodyOf(init)).toEqual({ redirectUrl: redirect });
      expect(chatgpt.getByRole("button", { name: "Sign out" })).toBeTruthy();
      expect(chatgpt.queryByRole("button", { name: "Sign in with ChatGPT" })).toBeNull();
      expect(screen.getByText("Using: ChatGPT subscription")).toBeTruthy();
    });

    test("shows the server's error when finishing sign-in is rejected", async () => {
      vi.spyOn(window, "open").mockReturnValue(null);
      routes["POST /api/agent/auth/chatgpt/complete"] = () =>
        json({ error: "That address doesn't contain a sign-in code" }, 400);
      renderPanel();
      const chatgpt = await waitFor(() => group("ChatGPT account"));
      fireEvent.click(chatgpt.getByRole("button", { name: "Sign in with ChatGPT" }));
      fireEvent.change(await chatgpt.findByLabelText("Paste the address from the sign-in tab"), {
        target: { value: "http://localhost:1455/nope" },
      });
      fireEvent.click(chatgpt.getByRole("button", { name: "Finish sign-in" }));

      expect(await chatgpt.findByText("That address doesn't contain a sign-in code")).toBeTruthy();
      expect(chatgpt.queryByText(/signed in as/i)).toBeNull();
    });

    test("finish sign-in stays disabled until an address is pasted", async () => {
      vi.spyOn(window, "open").mockReturnValue(null);
      renderPanel();
      const chatgpt = await waitFor(() => group("ChatGPT account"));
      fireEvent.click(chatgpt.getByRole("button", { name: "Sign in with ChatGPT" }));
      await chatgpt.findByLabelText("Paste the address from the sign-in tab");
      const finish = chatgpt.getByRole("button", { name: "Finish sign-in" }) as HTMLButtonElement;

      expect(finish.disabled).toBe(true);
      fireEvent.click(finish);
      expect(requests("POST", "/api/agent/auth/chatgpt/complete")).toHaveLength(0);
    });

    test("clicking sign-in twice while it is starting sends one request", async () => {
      const open = vi.spyOn(window, "open").mockReturnValue(null);
      const release = holdRoute("POST /api/agent/auth/chatgpt/start");
      renderPanel();
      const chatgpt = await waitFor(() => group("ChatGPT account"));
      const signIn = chatgpt.getByRole("button", { name: "Sign in with ChatGPT" });
      fireEvent.click(signIn);
      fireEvent.click(signIn);
      await release();

      await waitFor(() => expect(open).toHaveBeenCalledTimes(1));
      expect(requests("POST", "/api/agent/auth/chatgpt/start")).toHaveLength(1);
    });

    test("signing out forgets the account and offers sign-in again", async () => {
      auth = agentAuth({ using: "chatgpt", chatgpt: { signedIn: true, email: "jordan@example.com" } });
      renderPanel();
      const chatgpt = await waitFor(() => group("ChatGPT account"));
      await chatgpt.findByText("Signed in as jordan@example.com");
      fireEvent.click(chatgpt.getByRole("button", { name: "Sign out" }));

      expect(await chatgpt.findByRole("button", { name: "Sign in with ChatGPT" })).toBeTruthy();
      expect(requests("DELETE", "/api/agent/auth/chatgpt")).toHaveLength(1);
      expect(chatgpt.queryByText(/signed in as/i)).toBeNull();
      expect(screen.getByText("Not connected")).toBeTruthy();
    });

    test("tells you when OpenAI didn't confirm the sign-out", async () => {
      auth = agentAuth({ using: "chatgpt", chatgpt: { signedIn: true, email: "jordan@example.com" } });
      routes["DELETE /api/agent/auth/chatgpt"] = () =>
        json({ ...agentAuth(), warning: "OpenAI didn't confirm the sign-out; revoke access from your ChatGPT settings." });
      renderPanel();
      const chatgpt = await waitFor(() => group("ChatGPT account"));
      await chatgpt.findByText("Signed in as jordan@example.com");
      fireEvent.click(chatgpt.getByRole("button", { name: "Sign out" }));

      expect(await chatgpt.findByText(/didn't confirm the sign-out/)).toBeTruthy();
      expect(chatgpt.getByRole("button", { name: "Sign in with ChatGPT" })).toBeTruthy();
    });

    test("notes that the plan is the signed-in person's to use", async () => {
      renderPanel();
      const chatgpt = await waitFor(() => group("ChatGPT account"));
      expect(chatgpt.getByText(/uses your ChatGPT plan/i)).toBeTruthy();
    });
  });

  describe("OpenAI API key", () => {
    test("saving sends the key, reports it saved, and clears the field", async () => {
      renderPanel();
      const apiKey = await waitFor(() => group("OpenAI API key"));
      await apiKey.findByText("Not set");
      const input = apiKey.getByLabelText(/api key/i) as HTMLInputElement;
      expect(input.type).toBe("password");
      fireEvent.change(input, { target: { value: "sk-secret-123" } });
      fireEvent.click(apiKey.getByRole("button", { name: "Save" }));

      expect(await apiKey.findByText("Saved")).toBeTruthy();
      const [, init] = requests("PUT", "/api/agent/auth/api-key")[0];
      expect(bodyOf(init)).toEqual({ key: "sk-secret-123" });
      expect(input.value).toBe("");
      expect(screen.getByText("Using: API key")).toBeTruthy();
    });

    test("clearing removes the saved key", async () => {
      auth = agentAuth({ using: "apiKey", apiKey: { set: true } });
      renderPanel();
      const apiKey = await waitFor(() => group("OpenAI API key"));
      await apiKey.findByText("Saved");
      fireEvent.click(apiKey.getByRole("button", { name: "Clear" }));

      expect(await apiKey.findByText("Not set")).toBeTruthy();
      expect(requests("DELETE", "/api/agent/auth/api-key")).toHaveLength(1);
      expect(screen.getByText("Not connected")).toBeTruthy();
    });

    test("clicking save twice while saving sends one request", async () => {
      const release = holdRoute("PUT /api/agent/auth/api-key");
      renderPanel();
      const apiKey = await waitFor(() => group("OpenAI API key"));
      fireEvent.change(apiKey.getByLabelText(/api key/i), { target: { value: "sk-secret-123" } });
      const save = apiKey.getByRole("button", { name: "Save" });
      fireEvent.click(save);
      fireEvent.click(save);
      await release();

      expect(await apiKey.findByText("Saved")).toBeTruthy();
      expect(requests("PUT", "/api/agent/auth/api-key")).toHaveLength(1);
    });

    test("shows the server's error when the key is rejected", async () => {
      routes["PUT /api/agent/auth/api-key"] = () => json({ error: "key must not be empty" }, 400);
      renderPanel();
      const apiKey = await waitFor(() => group("OpenAI API key"));
      const input = (await apiKey.findByLabelText(/api key/i)) as HTMLInputElement;
      fireEvent.change(input, { target: { value: "   " } });
      fireEvent.click(apiKey.getByRole("button", { name: "Save" }));

      expect(await apiKey.findByText("key must not be empty")).toBeTruthy();
      expect(apiKey.queryByText("Saved")).toBeNull();
    });

    test("is locked when the key comes from the environment", async () => {
      auth = agentAuth({ using: "apiKey", apiKey: { set: true, fromEnv: true } });
      renderPanel();
      const apiKey = await waitFor(() => group("OpenAI API key"));
      expect(await apiKey.findByText("Set by environment (OPENAI_API_KEY)")).toBeTruthy();
      expect((apiKey.getByLabelText(/api key/i) as HTMLInputElement).disabled).toBe(true);
      expect((apiKey.getByRole("button", { name: "Save" }) as HTMLButtonElement).disabled).toBe(true);
      expect((apiKey.getByRole("button", { name: "Clear" }) as HTMLButtonElement).disabled).toBe(true);
    });
  });

  describe("Google Calendar", () => {
    test("explains how to create the key, linking to the Google Cloud console pages", async () => {
      renderPanel();
      const google = await waitFor(() => group("Google Calendar"));
      fireEvent.click(google.getByText("How to get a service account key"));

      const hrefs = google.getAllByRole("link").map((a) => a.getAttribute("href"));
      expect(hrefs).toContain("https://console.cloud.google.com/apis/library/calendar-json.googleapis.com");
      expect(hrefs).toContain("https://console.cloud.google.com/iam-admin/serviceaccounts");
      expect(google.getByText(/Add key/)).toBeTruthy();
      expect(google.getByText(/Make changes to events/)).toBeTruthy();
    });

    const KEY_JSON = JSON.stringify({
      type: "service_account",
      client_email: SERVICE_ACCOUNT_EMAIL,
      private_key: "-----BEGIN PRIVATE KEY-----\nsecretstuff\n-----END PRIVATE KEY-----\n",
    });

    test("reports when no key is saved", async () => {
      renderPanel();
      const google = await waitFor(() => group("Google Calendar"));
      expect(await google.findByText("Not set")).toBeTruthy();
      expect(google.queryByText(/share each calendar/i)).toBeNull();
    });

    test("saving sends the JSON key, clears the field, and says which account to share with", async () => {
      renderPanel();
      const google = await waitFor(() => group("Google Calendar"));
      const textarea = (await google.findByLabelText("Service account JSON key")) as HTMLTextAreaElement;
      expect(textarea.tagName).toBe("TEXTAREA");
      fireEvent.change(textarea, { target: { value: KEY_JSON } });
      fireEvent.click(google.getByRole("button", { name: "Save" }));

      expect(
        await google.findByText(`Share each calendar with ${SERVICE_ACCOUNT_EMAIL} (Make changes to events)`),
      ).toBeTruthy();
      const [, init] = requests("PUT", "/api/agent/auth/google")[0];
      expect(bodyOf(init)).toEqual({ json: KEY_JSON });
      expect(textarea.value).toBe("");
    });

    test("shows the server's error when the JSON key is rejected", async () => {
      routes["PUT /api/agent/auth/google"] = () => json({ error: "not a service account key" }, 400);
      renderPanel();
      const google = await waitFor(() => group("Google Calendar"));
      fireEvent.change(await google.findByLabelText("Service account JSON key"), { target: { value: "{}" } });
      fireEvent.click(google.getByRole("button", { name: "Save" }));

      expect(await google.findByText("not a service account key")).toBeTruthy();
      expect(google.queryByText(/share each calendar/i)).toBeNull();
    });

    test("clearing removes the saved key", async () => {
      auth = agentAuth({ google: { set: true, serviceAccountEmail: SERVICE_ACCOUNT_EMAIL } });
      renderPanel();
      const google = await waitFor(() => group("Google Calendar"));
      await google.findByText(`Share each calendar with ${SERVICE_ACCOUNT_EMAIL} (Make changes to events)`);
      fireEvent.click(google.getByRole("button", { name: "Clear" }));

      expect(await google.findByText("Not set")).toBeTruthy();
      expect(google.queryByText(/share each calendar/i)).toBeNull();
      expect(requests("DELETE", "/api/agent/auth/google")).toHaveLength(1);
    });

    test("is locked when the key comes from the environment", async () => {
      auth = agentAuth({ google: { set: true, fromEnv: true, serviceAccountEmail: SERVICE_ACCOUNT_EMAIL } });
      renderPanel();
      const google = await waitFor(() => group("Google Calendar"));
      expect(await google.findByText("Set by environment (GOOGLE_SERVICE_ACCOUNT_JSON)")).toBeTruthy();
      expect((google.getByLabelText("Service account JSON key") as HTMLTextAreaElement).disabled).toBe(true);
      expect((google.getByRole("button", { name: "Save" }) as HTMLButtonElement).disabled).toBe(true);
      expect((google.getByRole("button", { name: "Clear" }) as HTMLButtonElement).disabled).toBe(true);
    });
  });
});

describe("Admin assistant tab", () => {
  test("assistant edits are saved with the config", async () => {
    const config: Config = buildConfig();
    render(
      <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
        <Admin live={buildLive(config)} />
      </MemoryRouter>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Assistant" }));
    fireEvent.click(screen.getByRole("checkbox", { name: /enable ziggy/i }));
    fireEvent.change(screen.getByRole("textbox", { name: /model/i }), {
      target: { value: "gpt-7" },
    });
    await act(async () => {
      fireEvent.click(within(screen.getByRole("contentinfo")).getByRole("button", { name: "Save" }));
    });

    const put = fetchMock.mock.calls.find(
      ([url, init]) => String(url).endsWith("/api/config") && init?.method === "PUT",
    );
    expect(put).toBeTruthy();
    const saved: Config = JSON.parse(String(put![1].body));
    expect(saved.agent).toEqual({ ...config.agent, enabled: true, model: "gpt-7" });
  });

  test("credentials save immediately without the main Save and without touching the config", async () => {
    render(
      <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
        <Admin live={buildLive(buildConfig())} />
      </MemoryRouter>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Assistant" }));
    const mainSave = within(screen.getByRole("contentinfo")).getByRole("button", { name: "Save" }) as HTMLButtonElement;
    expect(mainSave.disabled).toBe(true);
    const apiKey = await waitFor(() => group("OpenAI API key"));
    fireEvent.change(apiKey.getByLabelText(/api key/i), { target: { value: "sk-live-456" } });
    fireEvent.click(apiKey.getByRole("button", { name: "Save" }));

    expect(await apiKey.findByText("Saved")).toBeTruthy();
    expect(requests("PUT", "/api/agent/auth/api-key")).toHaveLength(1);
    expect(requests("PUT", "/api/config")).toHaveLength(0);
    expect(mainSave.disabled).toBe(true);
  });
});

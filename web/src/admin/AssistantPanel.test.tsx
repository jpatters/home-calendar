import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import AssistantPanel from "./AssistantPanel";
import Admin from "./Admin";
import { buildConfig, buildLive } from "../display/displayFixtures";
import type { Agent, AgentSnapshot, Config } from "../types";

function agentConfig(overrides: Partial<Agent> = {}): Agent {
  return {
    enabled: false,
    model: "gpt-6.1-terra",
    effort: "low",
    textInputEnabled: false,
    quietStart: "22:00",
    quietEnd: "07:00",
    ...overrides,
  };
}

function agentStatus(overrides: Partial<AgentSnapshot> = {}): AgentSnapshot {
  return {
    state: "idle",
    heard: "",
    reply: "",
    at: "2026-11-26T12:00:00Z",
    ready: true,
    calendarWritable: false,
    ...overrides,
  };
}

let status: AgentSnapshot = agentStatus();
const fetchMock = vi.fn();

// GET /api/agent answers with `status`; PUT /api/config echoes the body.
// Anything else 404s.
beforeEach(() => {
  status = agentStatus();
  fetchMock.mockImplementation(async (url: string, init?: RequestInit) => {
    const path = String(url);
    if (path.endsWith("/api/agent") && (init?.method ?? "GET") === "GET") {
      return new Response(JSON.stringify(status), { status: 200 });
    }
    if (path.endsWith("/api/config") && init?.method === "PUT") {
      return new Response(String(init.body), { status: 200 });
    }
    return new Response("not found", { status: 404 });
  });
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

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
  ])("editing %s propagates", (_name, edit, expected) => {
    const initial = agentConfig();
    const changed = renderPanel(initial);
    edit();
    expect(changed()).toEqual({ ...initial, ...expected });
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

  test("says the status is unavailable when it cannot be fetched", async () => {
    fetchMock.mockImplementation(async () => new Response("boom", { status: 500 }));
    renderPanel();
    expect(await screen.findByText("Status unavailable")).toBeTruthy();
    expect(screen.queryByText(/openai key:/i)).toBeNull();
  });

  test("shows the saved settings", () => {
    renderPanel(agentConfig({ model: "gpt-x", effort: "medium", quietStart: "23:00", quietEnd: "08:00" }));
    expect((screen.getByRole("textbox", { name: /model/i }) as HTMLInputElement).value).toBe("gpt-x");
    expect((screen.getByRole("combobox", { name: /reasoning effort/i }) as HTMLSelectElement).value).toBe("medium");
    expect((screen.getByLabelText(/quiet hours start/i) as HTMLInputElement).value).toBe("23:00");
    expect((screen.getByLabelText(/quiet hours end/i) as HTMLInputElement).value).toBe("08:00");
  });

  test("reports the OpenAI key as set and calendar changes as unavailable", async () => {
    status = agentStatus({ ready: true, calendarWritable: false });
    renderPanel();
    expect(await screen.findByText(/openai key: set \(OPENAI_API_KEY\)/i)).toBeTruthy();
    expect(
      await screen.findByText(/calendar changes: unavailable \(GOOGLE_SERVICE_ACCOUNT_JSON\)/i),
    ).toBeTruthy();
  });

  test("reports the OpenAI key as missing and calendar changes as available", async () => {
    status = agentStatus({ ready: false, calendarWritable: true });
    renderPanel();
    expect(await screen.findByText(/openai key: missing \(OPENAI_API_KEY\)/i)).toBeTruthy();
    expect(
      await screen.findByText(/calendar changes: available \(GOOGLE_SERVICE_ACCOUNT_JSON\)/i),
    ).toBeTruthy();
  });

  test("has no field for entering a secret", async () => {
    renderPanel();
    await screen.findByText(/openai key:/i);
    expect(screen.queryByLabelText(/key|secret|token|credential|service account/i)).toBeNull();
    expect(document.querySelector('input[type="password"]')).toBeNull();
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
      fireEvent.click(screen.getByRole("button", { name: "Save" }));
    });

    const put = fetchMock.mock.calls.find(
      ([url, init]) => String(url).endsWith("/api/config") && init?.method === "PUT",
    );
    expect(put).toBeTruthy();
    const saved: Config = JSON.parse(String(put![1].body));
    expect(saved.agent).toEqual({ ...config.agent, enabled: true, model: "gpt-7" });
  });
});

import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import AgentPanel from "./AgentPanel";
import type { AgentSnapshot } from "../../types";

type Settings = { textInputEnabled: boolean; quietStart: string; quietEnd: string };

function settings(overrides: Partial<Settings> = {}): Settings {
  return {
    textInputEnabled: false,
    quietStart: "22:00",
    quietEnd: "07:00",
    ...overrides,
  };
}

function snapshot(overrides: Partial<AgentSnapshot> = {}): AgentSnapshot {
  return {
    state: "idle",
    heard: "",
    reply: "",
    at: new Date().toISOString(),
    ready: true,
    calendarWritable: false,
    ...overrides,
  };
}

function advance(ms: number) {
  act(() => {
    vi.advanceTimersByTime(ms);
  });
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date(2026, 6, 15, 12, 0));
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("AgentPanel", () => {
  test("renders an idle assistant panel with no agent snapshot", () => {
    render(<AgentPanel agent={null} weather={null} settings={settings()} />);
    expect(screen.getByRole("region", { name: "Assistant" })).toBeTruthy();
    expect(screen.getByRole("img", { name: "Assistant is idle" })).toBeTruthy();
  });

  test("shows the reply in a speech bubble when a talking snapshot arrives", () => {
    const { rerender } = render(<AgentPanel agent={snapshot()} weather={null} settings={settings()} />);
    expect(screen.queryByText("Hello there!")).toBeNull();

    rerender(
      <AgentPanel agent={snapshot({ state: "talking", reply: "Hello there!" })} weather={null} settings={settings()} />,
    );

    expect(screen.getByText("Hello there!")).toBeTruthy();
    expect(screen.getByRole("img", { name: "Assistant is talking" })).toBeTruthy();
  });

  test("a short reply talks for the 2 second minimum, then returns to idle", () => {
    render(
      <AgentPanel agent={snapshot({ state: "talking", reply: "Hello there!" })} weather={null} settings={settings()} />,
    );

    advance(1900);
    expect(screen.getByText("Hello there!")).toBeTruthy();

    advance(200);
    expect(screen.queryByText("Hello there!")).toBeNull();
    expect(screen.getByRole("img", { name: "Assistant is idle" })).toBeTruthy();
  });

  test("talking time scales with reply length", () => {
    const reply = "a".repeat(100);
    render(<AgentPanel agent={snapshot({ state: "talking", reply })} weather={null} settings={settings()} />);

    advance(5900);
    expect(screen.getByText(reply)).toBeTruthy();

    advance(200);
    expect(screen.queryByText(reply)).toBeNull();
  });

  test("a very long reply talks for at most 12 seconds", () => {
    const reply = "b".repeat(400);
    render(<AgentPanel agent={snapshot({ state: "talking", reply })} weather={null} settings={settings()} />);

    advance(11900);
    expect(screen.getByText(reply)).toBeTruthy();

    advance(200);
    expect(screen.queryByText(reply)).toBeNull();
  });

  test("a new talking snapshot with the same reply restarts the talking time", () => {
    const { rerender } = render(
      <AgentPanel agent={snapshot({ state: "talking", reply: "Again." })} weather={null} settings={settings()} />,
    );
    advance(1500);

    rerender(
      <AgentPanel
        agent={snapshot({ state: "talking", reply: "Again.", at: new Date().toISOString() })}
        weather={null}
        settings={settings()}
      />,
    );
    advance(1500);

    expect(screen.getByText("Again.")).toBeTruthy();
  });

  test("a talking snapshot whose talk time already elapsed before mounting stays idle", () => {
    const at = new Date(Date.now() - 5000).toISOString();
    render(
      <AgentPanel agent={snapshot({ state: "talking", reply: "Old news.", at })} weather={null} settings={settings()} />,
    );
    expect(screen.queryByText("Old news.")).toBeNull();
    expect(screen.getByRole("img", { name: "Assistant is idle" })).toBeTruthy();
  });

  test("mounting partway through a talk window talks only for the remainder", () => {
    const at = new Date(Date.now() - 1500).toISOString();
    render(
      <AgentPanel agent={snapshot({ state: "talking", reply: "Halfway.", at })} weather={null} settings={settings()} />,
    );
    expect(screen.getByText("Halfway.")).toBeTruthy();

    advance(600);

    expect(screen.queryByText("Halfway.")).toBeNull();
  });

  test("after talking during quiet hours the assistant goes back to sleep", () => {
    vi.setSystemTime(new Date(2026, 6, 15, 23, 30));
    render(
      <AgentPanel agent={snapshot({ state: "talking", reply: "Goodnight." })} weather={null} settings={settings()} />,
    );
    expect(screen.getByRole("img", { name: "Assistant is talking" })).toBeTruthy();

    advance(2100);

    expect(screen.queryByText("Goodnight.")).toBeNull();
    expect(screen.getByRole("img", { name: "Assistant is sleeping" })).toBeTruthy();
  });

  test("falls asleep within a minute of quiet hours beginning while the panel is open", () => {
    vi.setSystemTime(new Date(2026, 6, 15, 21, 59, 59));
    render(<AgentPanel agent={snapshot()} weather={null} settings={settings()} />);
    expect(screen.getByRole("img", { name: "Assistant is idle" })).toBeTruthy();

    advance(60 * 1000);

    expect(screen.getByRole("img", { name: "Assistant is sleeping" })).toBeTruthy();
  });

  test("shows an ellipsis bubble while thinking", () => {
    render(
      <AgentPanel agent={snapshot({ state: "thinking", reply: "stale" })} weather={null} settings={settings()} />,
    );
    expect(screen.getByText("…")).toBeTruthy();
    expect(screen.queryByText("stale")).toBeNull();
    expect(screen.getByRole("img", { name: "Assistant is thinking" })).toBeTruthy();
  });

  test("looks confused on error and shows the apology", () => {
    render(
      <AgentPanel
        agent={snapshot({ state: "error", reply: "I'm not set up yet." })}
        weather={null}
        settings={settings()}
      />,
    );
    expect(screen.getByRole("img", { name: "Assistant is confused" })).toBeTruthy();
    expect(screen.getByText("I'm not set up yet.")).toBeTruthy();
  });

  test("recovers from an error once the apology has been shown", () => {
    render(
      <AgentPanel
        agent={snapshot({ state: "error", reply: "I'm not set up yet." })}
        weather={null}
        settings={settings()}
      />,
    );
    advance(2100);
    expect(screen.getByRole("img", { name: "Assistant is idle" })).toBeTruthy();
    expect(screen.queryByText("I'm not set up yet.")).toBeNull();
  });

  test("an error from before the page loaded is not shown", () => {
    const longAgo = new Date(Date.now() - 60 * 60 * 1000).toISOString();
    render(
      <AgentPanel
        agent={snapshot({ state: "error", reply: "Something broke.", at: longAgo })}
        weather={null}
        settings={settings()}
      />,
    );
    expect(screen.getByRole("img", { name: "Assistant is idle" })).toBeTruthy();
    expect(screen.queryByText("Something broke.")).toBeNull();
  });

  test("offers no way to type when text input is disabled", () => {
    render(<AgentPanel agent={snapshot()} weather={null} settings={settings({ textInputEnabled: false })} />);
    expect(screen.queryByRole("button", { name: "Talk to assistant" })).toBeNull();
    expect(screen.queryByRole("textbox")).toBeNull();
  });

  test("tapping the assistant opens a text input when enabled", () => {
    render(<AgentPanel agent={snapshot()} weather={null} settings={settings({ textInputEnabled: true })} />);
    expect(screen.queryByRole("textbox")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Talk to assistant" }));

    expect(screen.getByRole("textbox")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Send" })).toBeTruthy();
  });

  test("submitting sends the typed text and closes the input", async () => {
    const sent: string[] = [];
    const onSend = async (text: string) => {
      sent.push(text);
    };
    render(
      <AgentPanel
        agent={snapshot()}
        weather={null}
        settings={settings({ textInputEnabled: true })}
        onSend={onSend}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Talk to assistant" }));
    fireEvent.change(screen.getByRole("textbox"), { target: { value: "what's for dinner?" } });
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Send" }));
    });

    expect(sent).toEqual(["what's for dinner?"]);
    expect(screen.queryByRole("textbox")).toBeNull();
  });

  function openInput(onSend: (text: string) => Promise<void>) {
    render(
      <AgentPanel agent={snapshot()} weather={null} settings={settings({ textInputEnabled: true })} onSend={onSend} />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Talk to assistant" }));
  }

  test("pressing Enter submits the typed text", async () => {
    const sent: string[] = [];
    openInput(async (text) => {
      sent.push(text);
    });

    fireEvent.change(screen.getByRole("textbox"), { target: { value: "turn on the lights" } });
    await act(async () => {
      fireEvent.keyDown(screen.getByRole("textbox"), { key: "Enter" });
    });

    expect(sent).toEqual(["turn on the lights"]);
  });

  test("a whitespace-only message is not sent", async () => {
    const sent: string[] = [];
    openInput(async (text) => {
      sent.push(text);
    });

    fireEvent.change(screen.getByRole("textbox"), { target: { value: "   " } });
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Send" }));
    });

    expect(sent).toEqual([]);
  });
});

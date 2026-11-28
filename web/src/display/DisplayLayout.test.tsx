import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import Display from "./Display";
import { WIDGETS } from "./layout";
import { buildConfig, buildLive } from "./displayFixtures";

vi.mock("./CalendarView", () => ({
  default: () => <div data-testid="calendar-view" />,
}));

const fetchMock = vi.fn();

beforeEach(() => {
  fetchMock.mockImplementation(async () => new Response("{}", { status: 200 }));
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

// Mirrors dockview's SerializedDockview: a layout as the display saves it: one group per widget id in
// `groups`, each group listing the panels tabbed inside it (first is active).
function savedLayout(groups: string[][]) {
  const panels: Record<string, unknown> = {};
  groups.flat().forEach((id) => {
    const title = WIDGETS.find((w) => w.id === id)?.title ?? id;
    panels[id] = { id, contentComponent: id, title };
  });
  return {
    grid: {
      root: {
        type: "branch",
        data: groups.map((views, i) => ({
          type: "leaf",
          data: { views, activeView: views[0], id: String(i + 1) },
          size: 100,
        })),
        size: 1000,
      },
      width: 1000,
      height: 1000,
      orientation: "VERTICAL",
    },
    panels,
  };
}

function isShown(el: Element): boolean {
  for (let n: Element | null = el; n; n = n.parentElement) {
    if ((n as HTMLElement).style?.display === "none") return false;
  }
  return true;
}

function visibleTabs(): string[] {
  return screen
    .queryAllByRole("tab")
    .filter(isShown)
    .map((t) => t.textContent ?? "");
}

function layoutRequests() {
  return fetchMock.mock.calls.filter(([url]) => String(url).endsWith("/api/layout"));
}

describe("Display layout", () => {
  test("a saved layout is applied: a widget tabbed behind another is not shown", () => {
    const layout = savedLayout([["calendar"], ["clock", "weather"], ["tide"]]);
    const { container } = render(<Display live={buildLive(buildConfig({ layout }))} />);
    expect(container.querySelector(".clock-widget")).not.toBeNull();
    expect(container.querySelector(".weather-widget")).toBeNull();
    expect(container.querySelector(".tide-widget")).not.toBeNull();
  });

  test("widgets enabled but missing from the saved layout are still shown", () => {
    const layout = savedLayout([["calendar"], ["clock", "weather"]]);
    const { container } = render(<Display live={buildLive(buildConfig({ layout }))} />);
    expect(container.querySelector(".weather-widget")).toBeNull();
    expect(container.querySelector(".tide-widget")).not.toBeNull();
    expect(container.querySelector(".pool-widget")).not.toBeNull();
  });

  test("a widget in the saved layout is not shown once it is disabled", () => {
    const layout = savedLayout([["calendar"], ["clock", "weather"], ["hockey"]]);
    const { container } = render(
      <Display live={buildLive(buildConfig({ layout, hockeyEnabled: false }))} />,
    );
    expect(container.querySelector(".hockey-widget")).toBeNull();
    expect(container.querySelector(".clock-widget")).not.toBeNull();
    expect(container.querySelector(".weather-widget")).toBeNull();
  });

  test("a widget disabled by a config update disappears and the saved layout stays applied", () => {
    const layout = savedLayout([["calendar"], ["clock", "weather"], ["hockey"]]);
    const { container, rerender } = render(<Display live={buildLive(buildConfig({ layout }))} />);
    expect(container.querySelector(".hockey-widget")).not.toBeNull();
    rerender(<Display live={buildLive(buildConfig({ layout, hockeyEnabled: false }))} />);
    expect(container.querySelector(".hockey-widget")).toBeNull();
    expect(container.querySelector(".weather-widget")).toBeNull();
  });

  test("a saved layout naming a widget that no longer exists falls back to the default", () => {
    const layout = savedLayout([["calendar"], ["clock", "weather"], ["oldwidget"]]);
    const { container } = render(<Display live={buildLive(buildConfig({ layout }))} />);
    expect(container.querySelector(".clock-widget")).not.toBeNull();
    expect(container.querySelector(".weather-widget")).not.toBeNull();
  });

  test("a corrupt saved layout falls back to showing every enabled widget", () => {
    const layout = { grid: { root: { type: "leaf" } }, panels: { x: { contentComponent: "gone" } } };
    const { container } = render(<Display live={buildLive(buildConfig({ layout }))} />);
    for (const cls of [
      ".calendar-pane",
      ".clock-widget",
      ".weather-widget",
      ".snowday-widget",
      ".tide-widget",
      ".baseball-widget",
      ".hockey-widget",
      ".pool-widget",
      ".hottub-widget",
    ]) {
      expect(container.querySelector(cls), cls).not.toBeNull();
    }
  });

  test("a widget re-enabled by a config update reappears", () => {
    const layout = savedLayout([["calendar"], ["clock", "weather"]]);
    const { container, rerender } = render(
      <Display live={buildLive(buildConfig({ layout, poolEnabled: false }))} />,
    );
    expect(container.querySelector(".pool-widget")).toBeNull();
    rerender(<Display live={buildLive(buildConfig({ layout, poolEnabled: true }))} />);
    expect(container.querySelector(".pool-widget")).not.toBeNull();
    expect(container.querySelector(".weather-widget")).toBeNull();
  });

  test("a new layout pushed from the server is applied while locked", () => {
    const { container, rerender } = render(
      <Display live={buildLive(buildConfig({ layout: savedLayout([["clock"], ["weather"]]) }))} />,
    );
    expect(container.querySelector(".weather-widget")).not.toBeNull();
    rerender(
      <Display live={buildLive(buildConfig({ layout: savedLayout([["clock", "weather"]]) }))} />,
    );
    expect(container.querySelector(".weather-widget")).toBeNull();
  });

  test("an unconfigured snow day widget gets no panel", () => {
    const config = buildConfig();
    config.snowDay.url = "";
    render(<Display live={buildLive(config)} />);
    fireEvent.click(screen.getByRole("button", { name: /edit layout/i }));
    expect(visibleTabs()).not.toContain("Snow day");
    expect(visibleTabs()).toContain("Clock");
  });
});

describe("Display layout editing", () => {
  test("tabs are hidden while locked and shown in edit mode", () => {
    render(<Display live={buildLive(buildConfig())} />);
    expect(visibleTabs()).toEqual([]);

    fireEvent.click(screen.getByRole("button", { name: /edit layout/i }));

    const tabs = visibleTabs();
    expect(tabs).toContain("Clock");
    expect(tabs).toContain("Weather");
  });

  test("widgets cannot be closed from their tabs in edit mode", () => {
    render(<Display live={buildLive(buildConfig())} />);
    fireEvent.click(screen.getByRole("button", { name: /edit layout/i }));
    expect(visibleTabs()).toContain("Clock");
    expect(screen.queryAllByRole("button", { name: /close tab/i })).toEqual([]);
  });

  test("Done saves the layout to the server and locks again", async () => {
    render(<Display live={buildLive(buildConfig())} />);
    fireEvent.click(screen.getByRole("button", { name: /edit layout/i }));
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: /done/i }));
    });

    const reqs = layoutRequests();
    expect(reqs).toHaveLength(1);
    const [, init] = reqs[0];
    expect(init.method).toBe("PUT");
    expect(visibleTabs()).toEqual([]);
    expect(screen.getByRole("button", { name: /edit layout/i })).toBeTruthy();
  });

  test("Done saves the arrangement on screen, not the one it was loaded with", async () => {
    const tabbed = savedLayout([["calendar"], ["clock", "weather"]]);
    const { unmount } = render(<Display live={buildLive(buildConfig({ layout: tabbed }))} />);
    fireEvent.click(screen.getByRole("button", { name: /edit layout/i }));
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: /reset/i }));
    });
    fireEvent.click(screen.getByRole("button", { name: /edit layout/i }));
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: /done/i }));
    });
    const saved = JSON.parse(layoutRequests().at(-1)![1].body);
    expect(saved).not.toBeNull();
    unmount();

    const { container } = render(<Display live={buildLive(buildConfig({ layout: saved }))} />);
    expect(container.querySelector(".clock-widget")).not.toBeNull();
    expect(container.querySelector(".weather-widget")).not.toBeNull();
  });

  test("a failed save keeps the layout editable and says so", async () => {
    fetchMock.mockImplementation(async () => new Response("boom", { status: 500 }));
    render(<Display live={buildLive(buildConfig())} />);
    fireEvent.click(screen.getByRole("button", { name: /edit layout/i }));
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: /done/i }));
    });
    expect(screen.getByText(/couldn't save layout/i).getAttribute("role")).toBe("alert");
    expect(screen.getByRole("button", { name: /done/i })).toBeTruthy();
    expect(visibleTabs()).toContain("Clock");
  });

  test("Reset restores the default arrangement and clears the saved layout", async () => {
    const { container } = render(
      <Display
        live={buildLive(buildConfig({ layout: savedLayout([["clock", "weather"]]) }))}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: /edit layout/i }));
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: /reset/i }));
    });

    const reqs = layoutRequests();
    expect(reqs).toHaveLength(1);
    expect(reqs[0][1].method).toBe("PUT");
    expect(JSON.parse(reqs[0][1].body)).toBeNull();
    expect(container.querySelector(".weather-widget")).not.toBeNull();
    expect(visibleTabs()).toEqual([]);
  });

  test("a layout pushed from the server does not disturb an edit in progress", () => {
    const { container, rerender } = render(
      <Display live={buildLive(buildConfig({ layout: savedLayout([["clock"], ["weather"]]) }))} />,
    );
    fireEvent.click(screen.getByRole("button", { name: /edit layout/i }));
    rerender(
      <Display live={buildLive(buildConfig({ layout: savedLayout([["clock", "weather"]]) }))} />,
    );
    expect(container.querySelector(".weather-widget")).not.toBeNull();
  });
});

function converseRequests(): { conversation_id: string; query: string }[] {
  return fetchMock.mock.calls
    .filter(([url, init]) => String(url).endsWith("/api/agent/converse") && init?.method === "POST")
    .map(([, init]) => JSON.parse(String(init.body)));
}

async function sayToAssistant(text: string) {
  fireEvent.click(screen.getByRole("button", { name: /talk to assistant/i }));
  fireEvent.change(screen.getByRole("textbox", { name: /message/i }), { target: { value: text } });
  await act(async () => {
    fireEvent.click(screen.getByRole("button", { name: /^send$/i }));
  });
}

describe("Display assistant panel", () => {
  test("is not shown while the assistant is disabled", () => {
    render(<Display live={buildLive(buildConfig({ agentEnabled: false }))} />);
    expect(screen.queryByRole("region", { name: "Assistant" })).toBeNull();
  });

  test("is shown once the assistant is enabled", () => {
    const { rerender } = render(<Display live={buildLive(buildConfig({ agentEnabled: false }))} />);
    rerender(<Display live={buildLive(buildConfig({ agentEnabled: true }))} />);
    expect(screen.getByRole("region", { name: "Assistant" })).toBeTruthy();
  });

  test("is titled Ziggy in edit mode", () => {
    render(<Display live={buildLive(buildConfig({ agentEnabled: true }))} />);
    fireEvent.click(screen.getByRole("button", { name: /edit layout/i }));
    expect(visibleTabs()).toContain("Ziggy");
  });

  test("offers no text box when typing is turned off", () => {
    render(
      <Display live={buildLive(buildConfig({ agentEnabled: true, agentTextInputEnabled: false }))} />,
    );
    expect(screen.getByRole("region", { name: "Assistant" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: /talk to assistant/i })).toBeNull();
  });

  test("typed messages go to the assistant as one continuing conversation", async () => {
    render(
      <Display live={buildLive(buildConfig({ agentEnabled: true, agentTextInputEnabled: true }))} />,
    );

    await sayToAssistant("What's on tomorrow?");
    expect(screen.queryByRole("textbox", { name: /message/i })).toBeNull();
    await sayToAssistant("And the day after?");

    const reqs = converseRequests();
    expect(reqs.map((r) => r.query)).toEqual(["What's on tomorrow?", "And the day after?"]);
    expect(reqs[0].conversation_id).toBeTruthy();
    expect(reqs[1].conversation_id).toBe(reqs[0].conversation_id);
  });
});

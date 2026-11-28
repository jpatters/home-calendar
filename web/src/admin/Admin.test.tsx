import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import Admin from "./Admin";
import type { Config } from "../types";
import { buildConfig, buildLive } from "../display/displayFixtures";

const fetchMock = vi.fn();

beforeEach(() => {
  fetchMock.mockImplementation(async (_url: string, init?: RequestInit) =>
    new Response(init?.body ?? "{}", { status: 200 }),
  );
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

function renderAdmin(config: Config = buildConfig()) {
  return render(
    <MemoryRouter>
      <Admin live={buildLive(config)} />
    </MemoryRouter>,
  );
}

function openSection(name: string) {
  const nav = screen.getByRole("navigation", { name: /settings sections/i });
  fireEvent.click(within(nav).getByRole("button", { name }));
}

function requestsTo(path: string) {
  return fetchMock.mock.calls.filter(([url]) => String(url) === path);
}

describe("Admin navigation", () => {
  test("choosing a section in the sidebar shows that section", () => {
    renderAdmin();
    expect(screen.getByRole("heading", { name: /^calendars/i })).toBeTruthy();

    openSection("Weather");

    expect(screen.getByRole("heading", { name: /^weather/i })).toBeTruthy();
    expect(screen.queryByRole("heading", { name: /^calendars/i })).toBeNull();
  });

  test("the menu button opens the section menu and choosing a section closes it", () => {
    renderAdmin();
    const menu = screen.getByRole("button", { name: /^menu$/i });
    expect(menu.getAttribute("aria-expanded")).toBe("false");

    fireEvent.click(menu);
    expect(menu.getAttribute("aria-expanded")).toBe("true");

    openSection("Tide");
    expect(menu.getAttribute("aria-expanded")).toBe("false");
    expect(screen.getByRole("heading", { name: /^tides/i })).toBeTruthy();
  });

  test("Escape closes the open section menu", () => {
    renderAdmin();
    const menu = screen.getByRole("button", { name: /^menu$/i });
    fireEvent.click(menu);
    const nav = screen.getByRole("navigation", { name: /settings sections/i });
    fireEvent.keyDown(within(nav).getByRole("button", { name: "Weather" }), { key: "Escape" });
    expect(menu.getAttribute("aria-expanded")).toBe("false");
    expect(document.activeElement).toBe(menu);
  });
});

describe("Admin refresh", () => {
  test("a section's refresh button refreshes that integration", async () => {
    renderAdmin();
    openSection("Tide");
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: /refresh tide now/i }));
    });
    expect(requestsTo("/api/tide/refresh")).toHaveLength(1);
    expect(requestsTo("/api/tide/refresh")[0][1]?.method).toBe("POST");
  });

  test("only the open section's refresh button is offered", () => {
    renderAdmin();
    openSection("Weather");
    expect(screen.getByRole("button", { name: /refresh weather now/i })).toBeTruthy();
    expect(screen.queryByRole("button", { name: /refresh tide now/i })).toBeNull();
  });

  test("no refresh button for a disabled integration", () => {
    renderAdmin(buildConfig({ tideEnabled: false }));
    openSection("Tide");
    expect(screen.queryByRole("button", { name: /refresh tide now/i })).toBeNull();
  });

  test("no refresh button for an integration that is not configured", () => {
    renderAdmin({ ...buildConfig(), hockey: { enabled: true, teamAbbr: "", teamName: "" } });
    openSection("Hockey");
    expect(screen.queryByRole("button", { name: /refresh hockey now/i })).toBeNull();
  });

  test("enabling an integration without saving does not offer its refresh yet", () => {
    renderAdmin(buildConfig({ tideEnabled: false }));
    openSection("Tide");
    fireEvent.click(screen.getByLabelText(/show tide widget/i));
    expect(screen.queryByRole("button", { name: /refresh tide now/i })).toBeNull();
  });
});

describe("Admin save", () => {
  test("Save is disabled until something changes, then saves the config", async () => {
    renderAdmin();
    openSection("Pool");
    const save = screen.getByRole("button", { name: /^save$/i }) as HTMLButtonElement;
    expect(save.disabled).toBe(true);

    fireEvent.change(screen.getByLabelText(/device url/i), {
      target: { value: "http://10.0.0.9" },
    });
    expect(save.disabled).toBe(false);
    await act(async () => {
      fireEvent.click(save);
    });

    const puts = requestsTo("/api/config");
    expect(puts).toHaveLength(1);
    expect(puts[0][1]?.method).toBe("PUT");
    expect(JSON.parse(String(puts[0][1]?.body)).pool.deviceUrl).toBe("http://10.0.0.9");
  });

  test("edits in one section are kept when saving from another", async () => {
    renderAdmin();
    openSection("Pool");
    fireEvent.change(screen.getByLabelText(/device url/i), {
      target: { value: "http://10.0.0.9" },
    });
    openSection("Weather");
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: /^save$/i }));
    });
    const [put] = requestsTo("/api/config");
    expect(JSON.parse(String(put[1]?.body)).pool.deviceUrl).toBe("http://10.0.0.9");
  });
});

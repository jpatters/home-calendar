import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import MemoriesPanel from "./MemoriesPanel";
import Admin from "./Admin";
import { buildConfig, buildLive } from "../display/displayFixtures";
import type { Memory } from "../types";

type Handler = (init?: RequestInit, id?: string) => Response | Promise<Response>;

let memories: Memory[] = [];
let routes: Record<string, Handler> = {};
let nextId = 1;
const fetchMock = vi.fn();

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status });
}

function textOf(init?: RequestInit): string {
  return (JSON.parse(String(init?.body)) as { text: string }).text;
}

function memory(text: string, id = `m${nextId++}`): Memory {
  return { id, text, created: "2026-11-28T10:00:00Z", updated: "2026-11-28T10:00:00Z" };
}

// The server's memory endpoints, backed by `memories`; tests replace entries
// in `routes` to simulate rejections.
function defaultRoutes(): Record<string, Handler> {
  return {
    "GET /api/agent/memories": () => json({ memories }),
    "POST /api/agent/memories": (init) => {
      const m = memory(textOf(init).trim());
      memories = [...memories, m];
      return json(m);
    },
    "PUT /api/agent/memories/:id": (init, id) => {
      const found = memories.find((m) => m.id === id);
      if (!found) return json({ error: "That memory doesn't exist." }, 404);
      const updated = { ...found, text: textOf(init) };
      memories = memories.map((m) => (m.id === id ? updated : m));
      return json(updated);
    },
    "DELETE /api/agent/memories/:id": (_init, id) => {
      memories = memories.filter((m) => m.id !== id);
      return new Response(null, { status: 204 });
    },
  };
}

beforeEach(() => {
  nextId = 1;
  memories = [];
  routes = defaultRoutes();
  fetchMock.mockImplementation(async (url: string, init?: RequestInit) => {
    const path = new URL(String(url), "http://localhost").pathname;
    const method = init?.method ?? "GET";
    const item = path.match(/^\/api\/agent\/memories\/(.+)$/);
    const handler = item ? routes[`${method} /api/agent/memories/:id`] : routes[`${method} ${path}`];
    return handler ? handler(init, item?.[1]) : new Response("not found", { status: 404 });
  });
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

function row(text: string) {
  return within(screen.getByDisplayValue(text).closest("li") as HTMLElement);
}

describe("MemoriesPanel", () => {
  test("lists what Ziggy remembers, oldest first", async () => {
    memories = [memory("Emma has soccer on Tuesdays."), memory("Jordan likes the pool at 29 degrees.")];
    render(<MemoriesPanel />);

    const boxes = await screen.findAllByRole("textbox", { name: "Memory" });
    expect(boxes.map((b) => (b as HTMLTextAreaElement).value)).toEqual([
      "Emma has soccer on Tuesdays.",
      "Jordan likes the pool at 29 degrees.",
    ]);
  });

  test("says when Ziggy doesn't remember anything yet", async () => {
    render(<MemoriesPanel />);
    expect(await screen.findByText(/doesn't remember anything yet/i)).toBeTruthy();
  });

  test("adding a memory saves it and shows it in the list", async () => {
    render(<MemoriesPanel />);
    await screen.findByText(/doesn't remember anything yet/i);

    fireEvent.change(screen.getByRole("textbox", { name: /new memory/i }), {
      target: { value: "Biscuit's birthday is in March." },
    });
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: /add memory/i }));
    });

    expect(memories.map((m) => m.text)).toEqual(["Biscuit's birthday is in March."]);
    expect(screen.getByDisplayValue("Biscuit's birthday is in March.")).toBeTruthy();
    expect((screen.getByRole("textbox", { name: /new memory/i }) as HTMLTextAreaElement).value).toBe("");
  });

  test("a blank new memory can't be added", async () => {
    render(<MemoriesPanel />);
    await screen.findByText(/doesn't remember anything yet/i);

    fireEvent.change(screen.getByRole("textbox", { name: /new memory/i }), { target: { value: "   " } });

    expect((screen.getByRole("button", { name: /add memory/i }) as HTMLButtonElement).disabled).toBe(true);
  });

  test("editing a memory saves the new text", async () => {
    memories = [memory("Jordan likes the pool at 29 degrees.")];
    render(<MemoriesPanel />);
    const box = await screen.findByDisplayValue("Jordan likes the pool at 29 degrees.");

    fireEvent.change(box, { target: { value: "Jordan likes the pool at 30 degrees." } });
    await act(async () => {
      fireEvent.click(row("Jordan likes the pool at 30 degrees.").getByRole("button", { name: /save/i }));
    });

    expect(memories[0].text).toBe("Jordan likes the pool at 30 degrees.");
  });

  test("save is only offered once a memory has been changed", async () => {
    memories = [memory("Unchanged.")];
    render(<MemoriesPanel />);
    await screen.findByDisplayValue("Unchanged.");

    expect((row("Unchanged.").getByRole("button", { name: /save/i }) as HTMLButtonElement).disabled).toBe(true);
  });

  test("deleting a memory removes it", async () => {
    memories = [memory("Old fact."), memory("Kept fact.")];
    render(<MemoriesPanel />);
    await screen.findByDisplayValue("Old fact.");

    await act(async () => {
      fireEvent.click(row("Old fact.").getByRole("button", { name: /delete/i }));
    });

    expect(memories.map((m) => m.text)).toEqual(["Kept fact."]);
    expect(screen.queryByDisplayValue("Old fact.")).toBeNull();
  });

  test("shows the server's error when a change is rejected", async () => {
    memories = [memory("A fact.")];
    routes["PUT /api/agent/memories/:id"] = () => json({ error: "A memory can be at most 300 characters." }, 400);
    render(<MemoriesPanel />);
    const box = await screen.findByDisplayValue("A fact.");

    fireEvent.change(box, { target: { value: "x".repeat(400) } });
    await act(async () => {
      fireEvent.click(row("x".repeat(400)).getByRole("button", { name: /save/i }));
    });

    expect(await screen.findByText("A memory can be at most 300 characters.")).toBeTruthy();
    expect(memories[0].text).toBe("A fact.");
  });

  test("says so when the memories can't be loaded", async () => {
    routes["GET /api/agent/memories"] = () => json({ error: "boom" }, 500);
    render(<MemoriesPanel />);
    expect(await screen.findByText(/couldn't load/i)).toBeTruthy();
  });

  test("is reached from the admin sidebar and saves without the main Save", async () => {
    memories = [memory("Emma has soccer on Tuesdays.")];
    render(
      <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
        <Admin live={buildLive(buildConfig())} />
      </MemoryRouter>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Memories" }));
    await screen.findByDisplayValue("Emma has soccer on Tuesdays.");
    await act(async () => {
      fireEvent.click(row("Emma has soccer on Tuesdays.").getByRole("button", { name: /delete/i }));
    });

    expect(memories).toEqual([]);
    expect(fetchMock.mock.calls.some(([, init]) => init?.method === "PUT" && String(init?.body).includes('"calendars"'))).toBe(
      false,
    );
  });
});

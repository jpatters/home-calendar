import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import PoolModal from "./PoolModal";
import type { PoolSnapshot } from "../types";

function snapshot(overrides: Partial<PoolSnapshot> = {}): PoolSnapshot {
  return {
    updatedAt: "2026-08-06T12:00:00Z",
    temperatureC: 25.2,
    targetC: 27,
    minTargetC: 10,
    maxTargetC: 40,
    heating: true,
    ...overrides,
  };
}

const fetchMock = vi.fn();

beforeEach(() => {
  vi.useFakeTimers();
  fetchMock.mockImplementation(async (_url: string, init?: RequestInit) => {
    const { targetC } = JSON.parse(String(init?.body ?? "{}"));
    return new Response(JSON.stringify(snapshot({ targetC })), { status: 200 });
  });
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.clearAllMocks();
  vi.useRealTimers();
});

function targetRequests(): number[] {
  return fetchMock.mock.calls
    .filter(([url, init]) => String(url).endsWith("/api/pool/target") && init?.method === "POST")
    .map(([, init]) => JSON.parse(String(init.body)).targetC);
}

const raise = () => fireEvent.click(screen.getByRole("button", { name: /raise target/i }));
const lower = () => fireEvent.click(screen.getByRole("button", { name: /lower target/i }));

// Lets the debounce fire and the resulting request settle.
async function settle() {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(1000);
  });
}

describe("PoolModal", () => {
  test("shows the current temperature, heater state, and target", () => {
    render(<PoolModal pool={snapshot()} onClose={() => {}} />);
    expect(screen.getByRole("dialog", { name: /pool/i })).toBeTruthy();
    expect(screen.getByText("25.2°C")).toBeTruthy();
    expect(screen.getByText(/heating/i)).toBeTruthy();
    expect(screen.getByText("27.0°C")).toBeTruthy();
  });

  test("each raise tap shows the target half a degree higher immediately", () => {
    render(<PoolModal pool={snapshot()} onClose={() => {}} />);
    raise();
    expect(screen.getByText("27.5°C")).toBeTruthy();
    raise();
    expect(screen.getByText("28.0°C")).toBeTruthy();
    expect(screen.queryByText("27.0°C")).toBeNull();
  });

  test("each lower tap shows the target half a degree lower immediately", () => {
    render(<PoolModal pool={snapshot()} onClose={() => {}} />);
    lower();
    expect(screen.getByText("26.5°C")).toBeTruthy();
  });

  test("sends the final target once after a burst of taps", async () => {
    render(<PoolModal pool={snapshot()} onClose={() => {}} />);
    raise();
    raise();
    raise();
    lower();
    expect(targetRequests()).toEqual([]);
    await settle();
    expect(targetRequests()).toEqual([28]);
    expect(screen.getByText("28.0°C")).toBeTruthy();
  });

  test("sends nothing when taps cancel out", async () => {
    render(<PoolModal pool={snapshot()} onClose={() => {}} />);
    raise();
    lower();
    await settle();
    expect(targetRequests()).toEqual([]);
    expect(screen.getByText("27.0°C")).toBeTruthy();
  });

  test("cannot raise above the heater's maximum", () => {
    render(<PoolModal pool={snapshot({ targetC: 39.5 })} onClose={() => {}} />);
    const btn = screen.getByRole("button", { name: /raise target/i }) as HTMLButtonElement;
    expect(btn.disabled).toBe(false);
    raise();
    expect(screen.getByText("40.0°C")).toBeTruthy();
    expect(btn.disabled).toBe(true);
  });

  test("cannot lower below the heater's minimum", () => {
    render(<PoolModal pool={snapshot({ targetC: 10.5 })} onClose={() => {}} />);
    const btn = screen.getByRole("button", { name: /lower target/i }) as HTMLButtonElement;
    expect(btn.disabled).toBe(false);
    lower();
    expect(screen.getByText("10.0°C")).toBeTruthy();
    expect(btn.disabled).toBe(true);
  });

  test("a live target between half degrees snaps to the next step in the tap's direction", () => {
    const { unmount } = render(<PoolModal pool={snapshot({ targetC: 27.3 })} onClose={() => {}} />);
    expect(screen.getByText("27.3°C")).toBeTruthy();
    raise();
    expect(screen.getByText("27.5°C")).toBeTruthy();
    unmount();

    render(<PoolModal pool={snapshot({ targetC: 27.3 })} onClose={() => {}} />);
    lower();
    expect(screen.getByText("27.0°C")).toBeTruthy();
  });

  test("shows an error and reverts the target when the update fails", async () => {
    fetchMock.mockImplementation(async () => new Response("pool heater did not respond", { status: 502 }));
    render(<PoolModal pool={snapshot()} onClose={() => {}} />);
    raise();
    await settle();
    expect(screen.getByRole("alert").textContent).toMatch(/couldn.t update the target/i);
    expect(screen.getByText("27.0°C")).toBeTruthy();
    expect(screen.queryByText("27.5°C")).toBeNull();
  });

  test("keeps showing the sent target until the live snapshot catches up", async () => {
    const { rerender } = render(<PoolModal pool={snapshot()} onClose={() => {}} />);
    raise();
    await settle();
    expect(targetRequests()).toEqual([27.5]);
    expect(screen.getByText("27.5°C")).toBeTruthy();
    rerender(<PoolModal pool={snapshot({ targetC: 27.5 })} onClose={() => {}} />);
    expect(screen.getByText("27.5°C")).toBeTruthy();
  });

  test("follows the live snapshot when the heater reports a different target", async () => {
    const { rerender } = render(<PoolModal pool={snapshot()} onClose={() => {}} />);
    raise();
    await settle();
    rerender(<PoolModal pool={snapshot({ targetC: 30 })} onClose={() => {}} />);
    expect(screen.getByText("30.0°C")).toBeTruthy();
    expect(screen.queryByText("27.5°C")).toBeNull();
  });

  test("a stale live snapshot does not undo a tap that has not been sent yet", () => {
    const { rerender } = render(<PoolModal pool={snapshot()} onClose={() => {}} />);
    raise();
    rerender(<PoolModal pool={snapshot({ targetC: 27 })} onClose={() => {}} />);
    expect(screen.getByText("27.5°C")).toBeTruthy();
    expect(targetRequests()).toEqual([]);
  });

  test("a tap made while an earlier write is in flight is not undone by that write's echo", async () => {
    let finishFirst: (r: Response) => void = () => {};
    const respond = fetchMock.getMockImplementation()!;
    fetchMock.mockImplementationOnce(
      () => new Promise<Response>((resolve) => { finishFirst = resolve; }),
    );
    const { rerender } = render(<PoolModal pool={snapshot()} onClose={() => {}} />);
    raise();
    await settle(); // first write (27.5) is now in flight
    raise();
    expect(screen.getByText("28.0°C")).toBeTruthy();
    rerender(<PoolModal pool={snapshot({ targetC: 27.5 })} onClose={() => {}} />);
    expect(screen.getByText("28.0°C")).toBeTruthy();
    await act(async () => {
      finishFirst(await respond("/api/pool/target", { body: JSON.stringify({ targetC: 27.5 }) }));
    });
    await settle(); // second write (28) sent
    expect(targetRequests()).toEqual([27.5, 28]);
    expect(screen.getByText("28.0°C")).toBeTruthy();
  });

  test("closing before the debounce fires still sends the pending target", () => {
    let closed = 0;
    render(<PoolModal pool={snapshot()} onClose={() => { closed += 1; }} />);
    raise();
    fireEvent.click(screen.getByRole("button", { name: /close/i }));
    expect(targetRequests()).toEqual([27.5]);
    expect(closed).toBe(1);
  });

  test("calls onClose when the area outside the dialog is clicked", () => {
    let closed = 0;
    render(<PoolModal pool={snapshot()} onClose={() => { closed += 1; }} />);
    const backdrop = screen.getByRole("dialog").parentElement;
    if (!backdrop) throw new Error("dialog has no parent backdrop");
    fireEvent.click(backdrop);
    expect(closed).toBe(1);
  });
});

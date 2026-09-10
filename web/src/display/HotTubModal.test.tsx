import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import HotTubModal from "./HotTubModal";
import type { HotTubSnapshot } from "../types";

vi.mock("../api", () => ({
  setHotTubTarget: vi.fn(),
}));

import { setHotTubTarget } from "../api";

beforeEach(() => {
  vi.useFakeTimers();
  vi.mocked(setHotTubTarget).mockImplementation(async (targetF: number) => snapshot({ targetF }));
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  vi.useRealTimers();
});

function snapshot(overrides: Partial<HotTubSnapshot> = {}): HotTubSnapshot {
  return {
    updatedAt: "2026-09-10T12:00:00Z",
    temperatureF: 67,
    targetF: 102,
    minTargetF: 59,
    maxTargetF: 106,
    heating: true,
    ...overrides,
  };
}

const raise = () => fireEvent.click(screen.getByRole("button", { name: /raise target/i }));
const lower = () => fireEvent.click(screen.getByRole("button", { name: /lower target/i }));

// Lets the debounce fire and the resulting request settle.
async function settle() {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(1000);
  });
}

describe("HotTubModal", () => {
  test("shows the current temperature, heater state, and target", () => {
    render(<HotTubModal hottub={snapshot({ temperatureF: 67.6 })} onClose={() => {}} />);
    expect(screen.getByRole("dialog", { name: /hot tub/i })).toBeTruthy();
    expect(screen.getByText("68°F")).toBeTruthy();
    expect(screen.getByText(/heating/i)).toBeTruthy();
    expect(screen.getByText("102°F")).toBeTruthy();
  });

  test("each raise tap shows the target one degree higher immediately", () => {
    render(<HotTubModal hottub={snapshot()} onClose={() => {}} />);
    raise();
    expect(screen.getByText("103°F")).toBeTruthy();
    raise();
    expect(screen.getByText("104°F")).toBeTruthy();
    expect(screen.queryByText("102°F")).toBeNull();
  });

  test("each lower tap shows the target one degree lower immediately", () => {
    render(<HotTubModal hottub={snapshot()} onClose={() => {}} />);
    lower();
    expect(screen.getByText("101°F")).toBeTruthy();
  });

  test("sends the final target once after a burst of taps", async () => {
    render(<HotTubModal hottub={snapshot()} onClose={() => {}} />);
    raise();
    raise();
    raise();
    lower();
    expect(setHotTubTarget).not.toHaveBeenCalled();
    await settle();
    expect(setHotTubTarget).toHaveBeenCalledTimes(1);
    expect(setHotTubTarget).toHaveBeenCalledWith(104);
    expect(screen.getByText("104°F")).toBeTruthy();
  });

  test("sends nothing when taps cancel out", async () => {
    render(<HotTubModal hottub={snapshot()} onClose={() => {}} />);
    raise();
    lower();
    await settle();
    expect(setHotTubTarget).not.toHaveBeenCalled();
    expect(screen.getByText("102°F")).toBeTruthy();
  });

  test("cannot raise above the tub's maximum", () => {
    render(<HotTubModal hottub={snapshot({ targetF: 105 })} onClose={() => {}} />);
    const btn = screen.getByRole("button", { name: /raise target/i }) as HTMLButtonElement;
    expect(btn.disabled).toBe(false);
    raise();
    expect(screen.getByText("106°F")).toBeTruthy();
    expect(btn.disabled).toBe(true);
  });

  test("cannot lower below the tub's minimum", () => {
    render(<HotTubModal hottub={snapshot({ targetF: 59 })} onClose={() => {}} />);
    const btn = screen.getByRole("button", { name: /lower target/i }) as HTMLButtonElement;
    expect(btn.disabled).toBe(true);
  });

  test("shows an error and reverts the target when the update fails", async () => {
    vi.mocked(setHotTubTarget).mockRejectedValue(new Error("POST /api/hottub/target 502"));
    render(<HotTubModal hottub={snapshot()} onClose={() => {}} />);
    raise();
    await settle();
    expect(screen.getByText(/couldn.t update the target/i)).toBeTruthy();
    expect(screen.getByText("102°F")).toBeTruthy();
    expect(screen.queryByText("103°F")).toBeNull();
  });

  test("keeps showing the sent target until the live snapshot catches up", async () => {
    const { rerender } = render(<HotTubModal hottub={snapshot()} onClose={() => {}} />);
    raise();
    await settle();
    expect(setHotTubTarget).toHaveBeenCalledWith(103);
    expect(screen.getByText("103°F")).toBeTruthy();
    rerender(<HotTubModal hottub={snapshot({ targetF: 103 })} onClose={() => {}} />);
    expect(screen.getByText("103°F")).toBeTruthy();
  });

  test("follows the live snapshot when the tub reports a different target", async () => {
    const { rerender } = render(<HotTubModal hottub={snapshot()} onClose={() => {}} />);
    raise();
    await settle();
    rerender(<HotTubModal hottub={snapshot({ targetF: 104 })} onClose={() => {}} />);
    expect(screen.getByText("104°F")).toBeTruthy();
    expect(screen.queryByText("103°F")).toBeNull();
  });

  test("a stale live snapshot does not undo a tap that has not been sent yet", () => {
    const { rerender } = render(<HotTubModal hottub={snapshot()} onClose={() => {}} />);
    raise();
    rerender(<HotTubModal hottub={snapshot({ targetF: 102 })} onClose={() => {}} />);
    expect(screen.getByText("103°F")).toBeTruthy();
    expect(setHotTubTarget).not.toHaveBeenCalled();
  });

  test("a tap made while an earlier write is in flight is not undone by that write's echo", async () => {
    let finishFirst: (s: HotTubSnapshot) => void = () => {};
    vi.mocked(setHotTubTarget).mockImplementationOnce(
      () => new Promise<HotTubSnapshot>((resolve) => { finishFirst = resolve; }),
    );
    const { rerender } = render(<HotTubModal hottub={snapshot()} onClose={() => {}} />);
    raise();
    await settle(); // first write (103) is now in flight
    raise();
    expect(screen.getByText("104°F")).toBeTruthy();
    rerender(<HotTubModal hottub={snapshot({ targetF: 103 })} onClose={() => {}} />);
    expect(screen.getByText("104°F")).toBeTruthy();
    await act(async () => { finishFirst(snapshot({ targetF: 103 })); });
    await settle(); // second write (104) sent
    expect(setHotTubTarget).toHaveBeenLastCalledWith(104);
    expect(screen.getByText("104°F")).toBeTruthy();
  });

  test("closing before the debounce fires still sends the pending target", () => {
    let closed = 0;
    render(<HotTubModal hottub={snapshot()} onClose={() => { closed += 1; }} />);
    raise();
    fireEvent.click(screen.getByRole("button", { name: /close/i }));
    expect(setHotTubTarget).toHaveBeenCalledTimes(1);
    expect(setHotTubTarget).toHaveBeenCalledWith(103);
    expect(closed).toBe(1);
  });

  test("calls onClose when the area outside the dialog is clicked", () => {
    let closed = 0;
    render(<HotTubModal hottub={snapshot()} onClose={() => { closed += 1; }} />);
    const backdrop = screen.getByRole("dialog").parentElement;
    if (!backdrop) throw new Error("dialog has no parent backdrop");
    fireEvent.click(backdrop);
    expect(closed).toBe(1);
  });
});

import { useEffect, useRef, useState } from "react";

// Taps within this window collapse into a single request carrying the final
// target, so a burst of taps is one write to the device instead of many.
const SEND_DELAY_MS = 600;

interface Options {
  live: number;
  min: number;
  max: number;
  step: (current: number, delta: 1 | -1) => number;
  write: (target: number) => Promise<unknown>;
}

// Local setpoint editing for a device that reports its target over the live
// feed: the chosen value shows immediately and is written once after the
// debounce, then the live value takes over again.
export function useSetpoint({ live, min, max, step, write }: Options) {
  // The target chosen here, shown in place of the live value until the
  // device reports a change of its own.
  const [chosen, setChosen] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const inFlight = useRef(0);
  const liveRef = useRef(live);
  liveRef.current = live;
  const target = chosen ?? live;

  // A live change while a tap is waiting to be sent, or a write is still in
  // flight, may just be the echo of an earlier write; the newest tap wins.
  useEffect(() => {
    if (timer.current === null && inFlight.current === 0) {
      setChosen(null);
    }
  }, [live]);

  const cancelTimer = () => {
    if (timer.current !== null) {
      clearTimeout(timer.current);
      timer.current = null;
    }
  };

  const send = (value: number) => {
    cancelTimer();
    if (value === liveRef.current) {
      setChosen(null);
      return;
    }
    inFlight.current += 1;
    write(value).then(
      () => {
        inFlight.current -= 1;
      },
      () => {
        inFlight.current -= 1;
        setChosen(null);
        setError("Couldn't update the target. Try again.");
      },
    );
  };

  const adjust = (delta: 1 | -1) => {
    const next = Math.min(max, Math.max(min, step(target, delta)));
    setChosen(next);
    setError(null);
    cancelTimer();
    timer.current = setTimeout(() => send(next), SEND_DELAY_MS);
  };

  // Sends a pending tap now rather than dropping it.
  const flush = () => {
    if (timer.current !== null && chosen !== null) {
      send(chosen);
    }
  };

  useEffect(() => cancelTimer, []);

  return { target, error, canRaise: target < max, canLower: target > min, adjust, flush };
}

import { useRef } from "react";
import type { TouchEvent } from "react";

interface SwipeOptions {
  onNext: () => void;
  onPrev: () => void;
  // Minimum horizontal distance in px to count as a swipe.
  threshold?: number;
  // Horizontal motion must exceed vertical motion by this factor to count.
  horizontalRatio?: number;
}

interface SwipeHandlers {
  onTouchStart: (e: TouchEvent) => void;
  onTouchEnd: (e: TouchEvent) => void;
}

export function useSwipe({
  onNext,
  onPrev,
  threshold = 50,
  horizontalRatio = 1.5,
}: SwipeOptions): SwipeHandlers {
  const start = useRef<{ x: number; y: number } | null>(null);

  return {
    onTouchStart: (e) => {
      const t = e.touches[0];
      if (!t) return;
      start.current = { x: t.clientX, y: t.clientY };
    },
    onTouchEnd: (e) => {
      const s = start.current;
      start.current = null;
      if (!s) return;
      const t = e.changedTouches[0];
      if (!t) return;
      const dx = t.clientX - s.x;
      const dy = t.clientY - s.y;
      if (Math.abs(dx) < threshold) return;
      if (Math.abs(dx) < Math.abs(dy) * horizontalRatio) return;
      if (dx < 0) onNext();
      else onPrev();
    },
  };
}

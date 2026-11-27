import { useEffect, type RefObject } from "react";

// Dockview moves a panel's DOM node when the layout tree is restructured (for
// example when a widget is dropped beside it). Browsers reset the scroll
// position of detached elements, so without this a scrolled panel jumps back
// to the top whenever the layout changes around it.
export function usePreservedScroll(ref: RefObject<HTMLElement>): void {
  useEffect(() => {
    const root = ref.current;
    if (!root) return;
    const positions = new Map<Element, number>();

    const remember = (e: Event) => {
      const el = e.target;
      if (el instanceof Element && root.isConnected) positions.set(el, el.scrollTop);
    };
    const restore = () => {
      for (const [el, top] of positions) {
        if (!el.isConnected) positions.delete(el);
        else if (el.scrollTop !== top) el.scrollTop = top;
      }
    };

    root.addEventListener("scroll", remember, true);
    const observer = new ResizeObserver(restore);
    observer.observe(root);
    return () => {
      root.removeEventListener("scroll", remember, true);
      observer.disconnect();
    };
  }, [ref]);
}

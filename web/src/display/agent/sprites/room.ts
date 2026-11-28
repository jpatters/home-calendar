// Ziggy's room, drawn as a list of pixel rectangles in a ROOM_W x ROOM_H grid.
// The window shows the sky and weather; precipitation animates with `tick`.
import type { Conditions, Sky } from "../scene";

export const ROOM_W = 112;
export const ROOM_H = 72;
export const FLOOR_Y = 54;
/** Where Ziggy's feet rest: the character's top row when standing. */
export const STAND_Y = FLOOR_Y + 6 - 28;
/** Horizontal range Ziggy wanders across (left edge of the sprite). */
export const WANDER_MIN_X = 44;
export const WANDER_MAX_X = 56;
/** Where Ziggy stands beside the bed before getting in. */
export const BEDSIDE_X = 58;
/** Where Ziggy stands to look out the window. */
export const LOOKOUT_X = 17;
/** Top-left of Ziggy's head on the pillow when asleep. */
export const SLEEP_X = 82;
export const SLEEP_Y = 31;

export interface Rect {
  x: number;
  y: number;
  w: number;
  h: number;
  fill: string;
  opacity?: number;
}

export interface RoomInput {
  sky: Sky;
  conditions: Conditions;
  tick: number;
  blanket: string;
}

const INK = "#2b2233";
const FRAME = "#f7f3ee";
const SKY: Record<Sky, [string, string]> = {
  day: ["#7cc8f5", "#c4ebff"],
  dusk: ["#6b5ba8", "#f6a96b"],
  night: ["#141a45", "#2c3577"],
};
const WIN = { x: 10, y: 9, w: 30, h: 26 };
const OVERCAST_GREY = "#8d939e";
/** Conditions that hide the sun, moon and stars behind cloud. */
const COVERED = new Set<Conditions>(["overcast", "rain", "snow", "fog", "thunder"]);

function seeded(seed: number): () => number {
  let s = seed;
  return () => (s = (s * 16807) % 2147483647) / 2147483647;
}
const starRand = seeded(7);
const STARS = Array.from({ length: 14 }, () => [
  Math.floor(starRand() * WIN.w),
  Math.floor(starRand() * WIN.h * 0.7),
]);
const dropRand = seeded(42);
const DROPS = Array.from({ length: 22 }, () => [dropRand() * WIN.w, dropRand() * WIN.h, 0.6 + dropRand() * 0.6]);

function mix(a: string, b: string, t: number): string {
  const pa = parseInt(a.slice(1), 16);
  const pb = parseInt(b.slice(1), 16);
  const ch = (s: number) => Math.round(((pa >> s) & 255) * (1 - t) + ((pb >> s) & 255) * t);
  return `#${((ch(16) << 16) | (ch(8) << 8) | ch(0)).toString(16).padStart(6, "0")}`;
}

export function roomRects({ sky, conditions, tick, blanket }: RoomInput): Rect[] {
  const out: Rect[] = [];
  const px = (x: number, y: number, w: number, h: number, fill: string, opacity?: number) =>
    out.push({ x, y, w, h, fill, opacity });
  const outline = (x: number, y: number, w: number, h: number) => {
    px(x - 1, y - 1, w + 2, 1, INK);
    px(x - 1, y + h, w + 2, 1, INK);
    px(x - 1, y, 1, h, INK);
    px(x + w, y, 1, h, INK);
  };

  // wall
  px(0, 0, ROOM_W, FLOOR_Y, "#cfe0cf");
  for (let i = 0; i < ROOM_W; i += 8) px(i, 0, 1, FLOOR_Y, "#c1d6c1");

  // window: sky
  const { x, y, w, h } = WIN;
  const covered = COVERED.has(conditions);
  const [top, bottom] = SKY[sky].map((c) => (covered ? mix(c, OVERCAST_GREY, 0.6) : c));
  for (let i = 0; i < h; i++) px(x, y + i, w, 1, mix(top, bottom, i / (h - 1)));
  const cloud = (cx: number, cy: number, fill: string) => {
    px(x + cx, y + cy + 2, 11, 3, fill);
    px(x + cx + 2, y + cy, 6, 2, fill);
  };
  if (!covered && sky === "night") {
    for (const [sx, sy] of STARS) px(x + sx, y + sy, 1, 1, (tick + sx) % 23 === 0 ? "#9aa4ff" : "#ffffff");
    px(x + 21, y + 4, 4, 4, "#f5f0d0");
    px(x + 23, y + 4, 2, 2, SKY.night[0]);
    if (conditions === "partly") cloud(17, 6, "#5a628f");
  } else if (!covered && sky === "day") {
    px(x + 4, y + 4, 5, 5, "#ffd84a");
    px(x + 3, y + 5, 7, 3, "#ffd84a");
    px(x + 5, y + 3, 3, 7, "#ffd84a");
    px(x + 15, y + 8, 9, 3, "#ffffff");
    px(x + 17, y + 6, 5, 2, "#ffffff");
    if (conditions === "partly") {
      cloud(4, 7, "#ffffff");
      cloud(17, 14, "#eef2f7");
    }
  } else if (!covered && sky === "dusk") {
    px(x + 8, y + h - 7, 8, 3, "#ffcf6e");
    px(x + 9, y + h - 9, 6, 2, "#ffcf6e");
    if (conditions === "partly") cloud(16, 5, "#c79bb0");
  }

  // window: weather
  if (conditions === "overcast" || conditions === "rain" || conditions === "snow" || conditions === "thunder") {
    const fill = conditions === "thunder" ? "#5f6273" : sky === "night" ? "#4b5065" : "#b7bcc8";
    px(x, y, w, 4, fill);
    px(x + 3, y + 4, 10, 2, fill);
    px(x + 16, y + 4, 11, 2, fill);
    if (conditions === "overcast") {
      cloud(1, 10, fill);
      cloud(16, 15, fill);
    }
  }
  if (conditions === "rain" || conditions === "thunder") {
    for (const [dx0, dy0, speed] of DROPS) {
      const dy = (dy0 + tick * 2.2 * speed) % (h - 5);
      const dx = (dx0 + tick * 0.6) % w;
      px(x + Math.floor(dx), y + 5 + Math.floor(dy), 1, 2, "#dbe9ff");
    }
  }
  if (conditions === "snow") {
    for (const [dx0, dy0, speed] of DROPS) {
      const dy = (dy0 + tick * 0.5 * speed) % (h - 5);
      const dx = (dx0 + Math.sin((tick + dy0) / 6) * 1.5 + w) % w;
      px(x + Math.floor(dx), y + 5 + Math.floor(dy), 1, 1, "#ffffff");
    }
    px(x, y + h - 2, w, 2, "#ffffff");
  }
  if (conditions === "fog") {
    for (let i = 0; i < 4; i++) px(x, y + 5 + i * 6 + (((tick >> 3) + i) % 2), w, 3, "#e8ebf0", 0.55);
  }
  if (conditions === "thunder" && tick % 40 < 2) px(x, y, w, h, "#ffffff");

  // window: frame and sill
  px(x - 2, y - 2, w + 4, 2, FRAME);
  px(x - 2, y + h, w + 4, 2, FRAME);
  px(x - 2, y, 2, h, FRAME);
  px(x + w, y, 2, h, FRAME);
  px(x + w / 2 - 1, y, 2, h, FRAME);
  px(x, y + h / 2 - 1, w, 2, FRAME);
  px(x - 4, y + h + 2, w + 8, 2, "#c9b8a3");
  outline(x - 2, y - 2, w + 4, h + 4);

  // wall calendar
  const cx = 50;
  const cy = 12;
  px(cx, cy, 14, 12, "#ffffff");
  px(cx, cy, 14, 3, "#d83b3b");
  outline(cx, cy, 14, 12);
  for (let r = 0; r < 3; r++) for (let c = 0; c < 4; c++) px(cx + 2 + c * 3, cy + 5 + r * 2, 2, 1, "#b7bcc8");
  px(cx + 5, cy + 7, 2, 1, "#d83b3b");
  px(cx + 6, cy - 2, 2, 2, INK);

  // floor and rug
  px(0, FLOOR_Y, ROOM_W, ROOM_H - FLOOR_Y, "#a8714a");
  for (let i = 0; i < 4; i++) px(0, FLOOR_Y + 4 + i * 5, ROOM_W, 1, "#8e5c3a");
  for (let i = 0; i < 6; i++) px((i * 23 + (i % 2) * 9) % ROOM_W, FLOOR_Y + 5 + (i % 3) * 5, 1, 4, "#8e5c3a");
  px(0, FLOOR_Y - 2, ROOM_W, 2, "#7a4a2a");
  px(38, FLOOR_Y + 8, 36, 6, "#d98b8b");
  px(40, FLOOR_Y + 9, 32, 4, "#e9a6a6");

  // bed
  const bx = 74;
  const by = 40;
  px(bx + 32, by - 8, 4, 22, "#7a4a2a");
  outline(bx + 32, by - 8, 4, 22);
  px(bx, by + 4, 32, 6, FRAME);
  px(bx, by + 10, 32, 4, "#7a4a2a");
  px(bx + 1, by + 14, 2, 3, "#5c3820");
  px(bx + 29, by + 14, 2, 3, "#5c3820");
  px(bx + 22, by, 10, 5, "#ffffff");
  outline(bx + 22, by, 10, 5);
  px(bx, by + 3, 22, 7, blanket);
  for (let i = 0; i < 22; i += 4) px(bx + i, by + 5, 2, 2, "#ffffff");
  outline(bx, by + 3, 32, 11);

  return out;
}

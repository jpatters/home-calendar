// Placeholder sprites. Every sprite is string rows, one char per pixel, "." transparent.
// Character frames and outfit overlays share one grid so overlays align with the body.
import type { OutfitId, Pose, Precip, Sky } from "../scene";

export const palette: Record<string, string> = {
  k: "#2b2135",
  s: "#f2c29b",
  b: "#4a7bd1",
  p: "#3b4a6b",
  r: "#d94848",
  g: "#3aa856",
  y: "#f4d03f",
  o: "#f08a24",
  w: "#ffffff",
  n: "#9b6b43",
  q: "#6c8ed6",
  l: "#8fd3ff",
  d: "#f6a46b",
  t: "#1b2350",
  f: "#c9c9c9",
  e: "#e9dcc4",
  h: "#b98a5e",
};

const body = [".ss.", "kssk", ".bb.", "bbbb", ".pp.", ".pp."];

export const poses: Record<Pose, string[][]> = {
  idle: [body, ["....", ".ss.", "kssk", ".bb.", "bbbb", ".pp."]],
  think: [[".ss.", "kss.", ".bbs", "bbb.", ".pp.", ".pp."]],
  talk: [body, [".ss.", "krrk", ".bb.", "bbbb", ".pp.", ".pp."]],
  sleep: [["......", ".ss...", "qqqqqq"]],
  confused: [[".kk.", "kssk", ".bb.", "bbbb", ".pp.", ".pp."]],
};

const hat = (c: string) => [c.repeat(4), "....", "....", "....", "....", "...."];
const shirt = (c: string) => ["....", "....", `.${c}${c}.`, c.repeat(4), "....", "...."];

export const outfits: Record<OutfitId, string[]> = {
  winter: ["rrrr", "....", ".yy.", "bbbb", "....", "...."],
  spring: shirt("y"),
  summer: ["....", "....", ".oo.", "oooo", ".ss.", "...."],
  fall: shirt("h"),
  newyear: hat("y"),
  valentines: shirt("r"),
  stpatricks: hat("g"),
  easter: ["w..w", "....", "....", "....", "....", "...."],
  canada: ["....", "....", ".rr.", "wrrw", "....", "...."],
  halloween: shirt("o"),
  thanksgiving: hat("k"),
  christmas: ["rrrw", "....", "....", "....", "....", "...."],
};

export const room = [
  "eeeeeeeeeeee",
  "eeeeeeeeeeee",
  "eeeeeeeeeeee",
  "eeeeeeeeeeee",
  "eeeeeeeeeeee",
  "nnnnnnnnnnnn",
];

export const bed = ["n....", "nqqqq", "n...n"];

export const windowSky: Record<Sky, string[]> = {
  day: ["lly", "lll"],
  dusk: ["ddd", "ddd"],
  night: ["tyt", "ttt"],
};

export const precip: Record<Precip, string[]> = {
  none: ["...", "..."],
  rain: ["q.q", ".q."],
  snow: ["w.w", ".w."],
  fog: ["fff", "..."],
  thunder: ["q.y", ".y."],
};

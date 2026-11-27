// Ziggy's sprite data. Every grid is 16 wide x 28 tall, one char per pixel,
// "." transparent. Rows 0-3 are headroom for hats; the head starts at row 4.
// Outfits recolor the shirt (t), pants (p) and shoes (b) and add overlays.
import type { OutfitId } from "../scene";

export const CHAR_W = 16;
export const CHAR_H = 28;
/** Rows of the grid that show above the blanket when Ziggy is in bed. */
export const SLEEP_VISIBLE_ROWS = 12;

const BASE_PALETTE: Record<string, string> = {
  k: "#2b2233",
  s: "#f4c9a3",
  h: "#6b3e26",
  c: "#f29b9b",
  m: "#a33b48",
  w: "#ffffff",
  r: "#d83b3b",
  g: "#2e8b57",
  G: "#1f5e3d",
  y: "#f2c14e",
  o: "#e8772e",
  q: "#f5a3c0",
  v: "#7b55c7",
};

const BODY = [
  "................",
  "................",
  "................",
  "................",
  "....kkkkkkkk....",
  "...khhhhhhhhk...",
  "..khhhhhhhhhhk..",
  "..khsssssssshk..",
  "..kssssssssssk..",
  "..kssksssskssk..",
  "..kssksssskssk..",
  "..kcssssssssck..",
  "..ksssskkssssk..",
  "...kssssssssk...",
  "....kkkkkkkk....",
  "......kssk......",
  "...kkttttttkk...",
  "..kttttttttttk..",
  ".kttkttttttkttk.",
  ".kttkttttttkttk.",
  ".ksskttttttkssk.",
  "..kkkttttttkkk..",
  "....kppppppk....",
  "....kppkkppk....",
  "....kppkkppk....",
  "....kppkkppk....",
  "...kbbbkkbbbk...",
  "...kkkkkkkkkk...",
];

type Layer = Record<number, string>;

export type Face = "idle" | "blink" | "talk" | "think" | "confused" | "sleep";

const FACES: Record<Face, Layer> = {
  idle: { 9: "..kssksssskssk..", 10: "..kssksssskssk..", 12: "..ksssskkssssk.." },
  blink: { 9: "..kssssssssssk..", 10: "..kssksssskssk..", 12: "..ksssskkssssk.." },
  talk: { 9: "..kssksssskssk..", 10: "..kssksssskssk..", 12: "..ksssmmmmsssk.." },
  think: { 9: "..kssksssskssk..", 10: "..kssssssssssk..", 12: "..ksssskkssssk.." },
  confused: { 9: "..kssksssskssk..", 10: "..kssksssskssk..", 12: "..kssskskssssk.." },
  sleep: { 9: "..kssssssssssk..", 10: "..kskksssskksk..", 12: "..ksssskkssssk.." },
};

const HATS = {
  toque: { 1: ".......ww.......", 2: "......rrrr......", 3: ".....rrrrrr.....", 4: "....rrrrrrrr....", 5: "...rrrrrrrrrr...", 6: "..wwwwwwwwwwww.." },
  santa: { 0: "...........ww...", 1: "........rrrrww..", 2: "......rrrrrr....", 3: ".....rrrrrrr....", 4: "....rrrrrrrr....", 5: "...rrrrrrrrrr...", 6: "..wwwwwwwwwwww.." },
  party: { 0: ".......yy.......", 1: ".......vv.......", 2: "......vyyv......", 3: "......vvvv......", 4: ".....vyyyyv.....", 5: ".....vvvvvv....." },
  tophat: { 0: "....GGGGGGGG....", 1: "....GGGGGGGG....", 2: "....GGGGGGGG....", 3: "....kkkyykkk....", 4: "....GGGGGGGG....", 5: "..GGGGGGGGGGGG.." },
  bunny: { 0: "....kk....kk....", 1: "...kwk....kwk...", 2: "...kqk....kqk...", 3: "...kqk....kqk...", 4: "...kwk....kwk..." },
  pilgrim: { 1: ".....kkkkkk.....", 2: ".....kkkkkk.....", 3: ".....kkkkkk.....", 4: ".....kyyyyk.....", 5: "...kkkkkkkkkk..." },
  stem: { 2: "........G.......", 3: ".......GG......." },
} satisfies Record<string, Layer>;

const MARKS = {
  scarf: { 15: ".....rrrrrr.....", 16: "...rrrrrrrrrr...", 17: "...rr..........." },
  heart: { 17: ".....rr.rr......", 18: ".....rrrrr......", 19: "......rrr.......", 20: ".......r........" },
  leaf: { 17: "........r.......", 18: "......rrrrr.....", 19: ".......rrr......", 20: "........r......." },
  jack: { 17: ".....kk..kk.....", 19: ".....k.kk.k.....", 20: "......kkkk......" },
  collar: { 16: "....wwwwwwww...." },
  summer: { 19: "..ss........ss..", 20: "..ss........ss..", 24: ".....ss..ss.....", 25: ".....ss..ss....." },
  buttons: { 17: ".......yy.......", 19: ".......yy......." },
} satisfies Record<string, Layer>;

interface Outfit {
  shirt: string;
  pants: string;
  shoes: string;
  hat?: keyof typeof HATS;
  marks?: (keyof typeof MARKS)[];
}

const OUTFITS: Record<OutfitId, Outfit> = {
  winter: { shirt: "#3b6fb6", pants: "#3a3f58", shoes: "#5a3b2a", hat: "toque", marks: ["scarf"] },
  spring: { shirt: "#f2c14e", pants: "#4f7cac", shoes: "#2e8b57", marks: ["buttons"] },
  summer: { shirt: "#ff8a65", pants: "#4f86c6", shoes: "#f2c14e", marks: ["summer"] },
  fall: { shirt: "#b5651d", pants: "#3a3f58", shoes: "#5a3b2a" },
  newyear: { shirt: "#5b3fa8", pants: "#2b2233", shoes: "#2b2233", hat: "party" },
  valentines: { shirt: "#f5a3c0", pants: "#3a3f58", shoes: "#5a3b2a", marks: ["heart"] },
  stpatricks: { shirt: "#2e8b57", pants: "#3a3f58", shoes: "#2b2233", hat: "tophat" },
  easter: { shirt: "#c9e7a7", pants: "#b9a3e3", shoes: "#f5a3c0", hat: "bunny" },
  canada: { shirt: "#ffffff", pants: "#d83b3b", shoes: "#2b2233", marks: ["leaf"] },
  thanksgiving: { shirt: "#7a4a2a", pants: "#2b2233", shoes: "#2b2233", hat: "pilgrim", marks: ["collar"] },
  halloween: { shirt: "#e8772e", pants: "#2b2233", shoes: "#2b2233", hat: "stem", marks: ["jack"] },
  christmas: { shirt: "#d83b3b", pants: "#2e8b57", shoes: "#2b2233", hat: "santa" },
};

export interface CharacterSprite {
  grid: string[];
  palette: Record<string, string>;
}

function overlay(rows: string[], layer: Layer): void {
  for (const [i, row] of Object.entries(layer)) {
    const idx = Number(i);
    rows[idx] = rows[idx]
      .split("")
      .map((ch, x) => (row[x] && row[x] !== "." ? row[x] : ch))
      .join("");
  }
}

export function composeCharacter(outfitId: OutfitId, face: Face): CharacterSprite {
  const outfit = OUTFITS[outfitId];
  const grid = BODY.slice();
  overlay(grid, FACES[face]);
  for (const mark of outfit.marks ?? []) overlay(grid, MARKS[mark]);
  if (outfit.hat) overlay(grid, HATS[outfit.hat]);
  return {
    grid,
    palette: { ...BASE_PALETTE, t: outfit.shirt, p: outfit.pants, b: outfit.shoes },
  };
}

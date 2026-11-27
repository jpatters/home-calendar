import type { AgentState, WeatherSnapshot } from "../../types";

export type Pose = "idle" | "think" | "talk" | "sleep" | "confused";
export type Sky = "day" | "dusk" | "night";
export type Precip = "none" | "rain" | "snow" | "fog" | "thunder";
export type OutfitId =
  | "winter"
  | "spring"
  | "summer"
  | "fall"
  | "newyear"
  | "valentines"
  | "stpatricks"
  | "easter"
  | "canada"
  | "halloween"
  | "thanksgiving"
  | "christmas";

export interface SceneInput {
  now: Date;
  weather: WeatherSnapshot | null;
  agentState: AgentState;
  quietStart: string;
  quietEnd: string;
}

export interface Scene {
  pose: Pose;
  outfit: OutfitId;
  sky: Sky;
  precip: Precip;
}

/** Easter Sunday (Gregorian) via the anonymous Meeus/Jones/Butcher algorithm. */
function easterSunday(year: number): Date {
  const a = year % 19;
  const b = Math.floor(year / 100);
  const c = year % 100;
  const d = Math.floor(b / 4);
  const e = b % 4;
  const f = Math.floor((b + 8) / 25);
  const g = Math.floor((b - f + 1) / 3);
  const h = (19 * a + b - d - g + 15) % 30;
  const i = Math.floor(c / 4);
  const k = c % 4;
  const l = (32 + 2 * e + 2 * i - h - k) % 7;
  const m = Math.floor((a + 11 * h + 22 * l) / 451);
  const month = Math.floor((h + l - 7 * m + 114) / 31);
  const day = ((h + l - 7 * m + 114) % 31) + 1;
  return new Date(year, month - 1, day);
}

/** Canadian Thanksgiving: the second Monday of October. */
function thanksgiving(year: number): Date {
  const firstWeekday = new Date(year, 9, 1).getDay();
  const firstMonday = 1 + ((8 - firstWeekday) % 7);
  return new Date(year, 9, firstMonday + 7);
}

const holidayOutfitsByYear = new Map<number, Map<string, OutfitId>>();

function holidayOutfits(year: number): Map<string, OutfitId> {
  let byDate = holidayOutfitsByYear.get(year);
  if (!byDate) {
    const at = (month: number, day: number) => localDate(new Date(year, month - 1, day));
    byDate = new Map<string, OutfitId>([
      [at(1, 1), "newyear"],
      [at(2, 14), "valentines"],
      [at(3, 17), "stpatricks"],
      [localDate(easterSunday(year)), "easter"],
      [at(7, 1), "canada"],
      [localDate(thanksgiving(year)), "thanksgiving"],
      [at(10, 31), "halloween"],
      [at(12, 25), "christmas"],
    ]);
    holidayOutfitsByYear.set(year, byDate);
  }
  return byDate;
}

/** A holiday outfit is worn on the holiday and this many days before it. */
const HOLIDAY_LEAD_DAYS = 2;

function holidayOutfitFor(now: Date): OutfitId | undefined {
  for (let ahead = 0; ahead <= HOLIDAY_LEAD_DAYS; ahead++) {
    const day = new Date(now.getFullYear(), now.getMonth(), now.getDate() + ahead);
    const outfit = holidayOutfits(day.getFullYear()).get(localDate(day));
    if (outfit) return outfit;
  }
  return undefined;
}

function pad(n: number): string {
  return String(n).padStart(2, "0");
}

function localDate(d: Date): string {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

function minutesOf(hhmm: string): number {
  const [h, m] = hhmm.split(":").map(Number);
  return h * 60 + m;
}

function seasonOutfit(month: number): OutfitId {
  if (month === 11 || month <= 1) return "winter";
  if (month <= 4) return "spring";
  if (month <= 7) return "summer";
  return "fall";
}

function inQuietHours(minute: number, start: number, end: number): boolean {
  if (start === end) return false;
  if (start < end) return minute >= start && minute < end;
  return minute >= start || minute < end;
}

function poseFor(state: AgentState, quiet: boolean): Pose {
  if (state === "thinking") return "think";
  if (state === "talking") return "talk";
  if (state === "error") return "confused";
  return quiet ? "sleep" : "idle";
}

const DUSK_MINUTES = 30;

function skyFor(minute: number, today: string, weather: WeatherSnapshot | null): Sky {
  const daily = weather?.daily.find((d) => d.date === today);
  if (!daily) return minute >= 7 * 60 && minute < 19 * 60 ? "day" : "night";
  const sunrise = minutesOf(daily.sunrise.slice(11));
  const sunset = minutesOf(daily.sunset.slice(11));
  if (Math.abs(minute - sunrise) <= DUSK_MINUTES || Math.abs(minute - sunset) <= DUSK_MINUTES) return "dusk";
  return minute > sunrise && minute < sunset ? "day" : "night";
}

function precipFor(code: number): Precip {
  if (code === 45 || code === 48) return "fog";
  if ((code >= 51 && code <= 67) || (code >= 80 && code <= 82)) return "rain";
  if ((code >= 71 && code <= 77) || code === 85 || code === 86) return "snow";
  if (code >= 95 && code <= 99) return "thunder";
  return "none";
}

export function sceneFor({ now, weather, agentState, quietStart, quietEnd }: SceneInput): Scene {
  const today = localDate(now);
  const minute = now.getHours() * 60 + now.getMinutes();
  const quiet = inQuietHours(minute, minutesOf(quietStart), minutesOf(quietEnd));
  return {
    pose: poseFor(agentState, quiet),
    outfit: holidayOutfitFor(now) ?? seasonOutfit(now.getMonth()),
    sky: skyFor(minute, today, weather),
    precip: weather ? precipFor(weather.current.weatherCode) : "none",
  };
}

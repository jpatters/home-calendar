import Holidays from "date-holidays";
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

const EASTER_SUNDAY = "easter";
const THANKSGIVING = "2nd monday after 10-01";

const holidays = new Holidays("CA");
holidays.setHoliday(EASTER_SUNDAY, { name: "Easter Sunday", type: "observance" });
holidays.setHoliday(THANKSGIVING, { name: "Thanksgiving", type: "observance" });

const OUTFIT_BY_RULE: Record<string, OutfitId> = {
  "01-01": "newyear",
  "02-14": "valentines",
  "03-17": "stpatricks",
  [EASTER_SUNDAY]: "easter",
  "07-01": "canada",
  "10-31 18:00": "halloween",
  [THANKSGIVING]: "thanksgiving",
  "12-25": "christmas",
};

const holidayOutfitsByYear = new Map<number, Map<string, OutfitId>>();

function holidayOutfits(year: number): Map<string, OutfitId> {
  let byDate = holidayOutfitsByYear.get(year);
  if (!byDate) {
    byDate = new Map();
    for (const h of holidays.getHolidays(year)) {
      const outfit = OUTFIT_BY_RULE[h.rule];
      if (outfit) byDate.set(h.date.slice(0, 10), outfit);
    }
    holidayOutfitsByYear.set(year, byDate);
  }
  return byDate;
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
    outfit: holidayOutfits(now.getFullYear()).get(today) ?? seasonOutfit(now.getMonth()),
    sky: skyFor(minute, today, weather),
    precip: weather ? precipFor(weather.current.weatherCode) : "none",
  };
}

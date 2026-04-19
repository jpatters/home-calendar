import { useEffect, useReducer } from "react";
import type { LiveData } from "./useLiveData";
import type { ThemeMode, ThemePalette, WeatherSnapshot } from "./types";

export const PALETTES: ThemePalette[] = ["default", "ocean", "sunset", "forest"];
export const MODES: ThemeMode[] = ["light", "dark", "auto"];

export const PALETTE_LABELS: Record<ThemePalette, string> = {
  default: "Default",
  ocean: "Ocean",
  sunset: "Sunset",
  forest: "Forest",
};

export const MODE_LABELS: Record<ThemeMode, string> = {
  light: "Light",
  dark: "Dark",
  auto: "Auto (sunrise/sunset)",
};

export function resolveMode(
  mode: ThemeMode,
  weather: WeatherSnapshot | null,
  now: Date,
): "light" | "dark" {
  if (mode === "light" || mode === "dark") return mode;
  if (!weather) return "light";
  const today = formatLocalDate(now);
  const daily = weather.daily.find((d) => d.date === today);
  if (!daily) return "light";
  const sunrise = new Date(daily.sunrise);
  const sunset = new Date(daily.sunset);
  if (Number.isNaN(sunrise.getTime()) || Number.isNaN(sunset.getTime())) return "light";
  return now >= sunrise && now < sunset ? "light" : "dark";
}

function formatLocalDate(d: Date): string {
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

export function useTheme(live: LiveData): void {
  const palette = live.config?.display.theme ?? null;
  const mode = live.config?.display.mode ?? null;
  const weather = live.weather;
  const [tick, tickNow] = useReducer((x: number) => x + 1, 0);

  useEffect(() => {
    if (mode !== "auto") return;
    const id = window.setInterval(tickNow, 60_000);
    return () => window.clearInterval(id);
  }, [mode]);

  useEffect(() => {
    if (!palette || !mode) return;
    const resolved = resolveMode(mode, weather, new Date());
    const root = document.documentElement;
    root.dataset.palette = palette;
    root.dataset.mode = resolved;
  }, [palette, mode, weather, tick]);
}

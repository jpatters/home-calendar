import type { BaseballTeam, Config, GeoResult } from "./types";

export async function getConfig(): Promise<Config> {
  const res = await fetch("/api/config");
  if (!res.ok) throw new Error(`GET /api/config ${res.status}`);
  return res.json();
}

export async function putConfig(c: Config): Promise<Config> {
  const res = await fetch("/api/config", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(c),
  });
  if (!res.ok) throw new Error(`PUT /api/config ${res.status}`);
  return res.json();
}

export async function refreshCalendars(): Promise<void> {
  await fetch("/api/calendar/refresh", { method: "POST" });
}

export async function refreshWeather(): Promise<void> {
  await fetch("/api/weather/refresh", { method: "POST" });
}

export async function refreshSnowDay(): Promise<void> {
  await fetch("/api/snowday/refresh", { method: "POST" });
}

export async function refreshTide(): Promise<void> {
  await fetch("/api/tide/refresh", { method: "POST" });
}

export async function refreshBaseball(): Promise<void> {
  await fetch("/api/baseball/refresh", { method: "POST" });
}

export async function geocode(query: string, signal?: AbortSignal): Promise<GeoResult[]> {
  const res = await fetch(`/api/weather/geocode?q=${encodeURIComponent(query)}`, { signal });
  if (!res.ok) throw new Error(`GET /api/weather/geocode ${res.status}`);
  const data = await res.json();
  return Array.isArray(data) ? data : [];
}

export async function baseballTeamSearch(query: string, signal?: AbortSignal): Promise<BaseballTeam[]> {
  const res = await fetch(`/api/baseball/teams?q=${encodeURIComponent(query)}`, { signal });
  if (!res.ok) throw new Error(`GET /api/baseball/teams ${res.status}`);
  const data = await res.json();
  return Array.isArray(data) ? data : [];
}

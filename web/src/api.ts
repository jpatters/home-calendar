import type {
  AgentSnapshot,
  BaseballTeam,
  Config,
  GeoResult,
  HockeyTeam,
  HotTubSnapshot,
  PoolSnapshot,
  TideStation,
} from "./types";

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

export async function putLayout(layout: object | null): Promise<void> {
  const res = await fetch("/api/layout", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(layout),
  });
  if (!res.ok) throw new Error(`PUT /api/layout ${res.status}`);
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

export async function refreshHockey(): Promise<void> {
  await fetch("/api/hockey/refresh", { method: "POST" });
}

export async function refreshPool(): Promise<void> {
  await fetch("/api/pool/refresh", { method: "POST" });
}

export async function refreshHotTub(): Promise<void> {
  await fetch("/api/hottub/refresh", { method: "POST" });
}

export async function setHotTubTarget(targetF: number): Promise<HotTubSnapshot> {
  const res = await fetch("/api/hottub/target", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ targetF }),
  });
  if (!res.ok) throw new Error(`POST /api/hottub/target ${res.status}`);
  return res.json();
}

export async function setPoolTarget(targetC: number): Promise<PoolSnapshot> {
  const res = await fetch("/api/pool/target", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ targetC }),
  });
  if (!res.ok) throw new Error(`POST /api/pool/target ${res.status}`);
  return res.json();
}

export async function getAgentStatus(): Promise<AgentSnapshot> {
  const res = await fetch("/api/agent");
  if (!res.ok) throw new Error(`GET /api/agent ${res.status}`);
  return res.json();
}

export async function geocode(query: string, signal?: AbortSignal): Promise<GeoResult[]> {
  const res = await fetch(`/api/weather/geocode?q=${encodeURIComponent(query)}`, { signal });
  if (!res.ok) throw new Error(`GET /api/weather/geocode ${res.status}`);
  const data = await res.json();
  return Array.isArray(data) ? data : [];
}

export async function tideStationSearch(query: string, signal?: AbortSignal): Promise<TideStation[]> {
  const res = await fetch(`/api/tide/stations?q=${encodeURIComponent(query)}`, { signal });
  if (!res.ok) throw new Error(`GET /api/tide/stations ${res.status}`);
  const data = await res.json();
  return Array.isArray(data) ? data : [];
}

export async function baseballTeamSearch(query: string, signal?: AbortSignal): Promise<BaseballTeam[]> {
  const res = await fetch(`/api/baseball/teams?q=${encodeURIComponent(query)}`, { signal });
  if (!res.ok) throw new Error(`GET /api/baseball/teams ${res.status}`);
  const data = await res.json();
  return Array.isArray(data) ? data : [];
}

export async function hockeyTeamSearch(query: string, signal?: AbortSignal): Promise<HockeyTeam[]> {
  const res = await fetch(`/api/hockey/teams?q=${encodeURIComponent(query)}`, { signal });
  if (!res.ok) throw new Error(`GET /api/hockey/teams ${res.status}`);
  const data = await res.json();
  return Array.isArray(data) ? data : [];
}

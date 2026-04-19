import type { Config } from "./types";

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

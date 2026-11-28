import type {
  AgentAuth,
  BaseballTeam,
  Config,
  GeoResult,
  HockeyTeam,
  HotTubSnapshot,
  Memory,
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

// The credential endpoints answer errors with {"error": "message"}; that
// message is what the admin shows, so it becomes the thrown error's message.
// Assistant endpoints answer errors as {"error": "..."}, which becomes the
// thrown Error's message.
async function agentRequest<T>(method: string, path: string, body?: object): Promise<T> {
  const res = await fetch(`/api/agent${path}`, {
    method,
    headers: body ? { "Content-Type": "application/json" } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) {
    const data = await res.json().catch(() => null);
    throw new Error(data?.error || `${method} /api/agent${path} ${res.status}`);
  }
  return res.status === 204 ? (undefined as T) : res.json();
}

function agentAuthRequest<T = AgentAuth>(method: string, path: string, body?: object): Promise<T> {
  return agentRequest<T>(method, `/auth${path}`, body);
}

export function getAgentAuth(): Promise<AgentAuth> {
  return agentAuthRequest("GET", "");
}

export function startChatGPTSignIn(): Promise<{ authorizeUrl: string }> {
  return agentAuthRequest("POST", "/chatgpt/start");
}

export function completeChatGPTSignIn(redirectUrl: string): Promise<AgentAuth> {
  return agentAuthRequest("POST", "/chatgpt/complete", { redirectUrl });
}

export function signOutChatGPT(): Promise<AgentAuth> {
  return agentAuthRequest("DELETE", "/chatgpt");
}

export function putAgentApiKey(key: string): Promise<AgentAuth> {
  return agentAuthRequest("PUT", "/api-key", { key });
}

export function clearAgentApiKey(): Promise<AgentAuth> {
  return agentAuthRequest("DELETE", "/api-key");
}

export function putAgentGoogle(json: string): Promise<AgentAuth> {
  return agentAuthRequest("PUT", "/google", { json });
}

export function clearAgentGoogle(): Promise<AgentAuth> {
  return agentAuthRequest("DELETE", "/google");
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

export async function listMemories(): Promise<Memory[]> {
  return (await agentRequest<{ memories: Memory[] }>("GET", "/memories")).memories;
}

export function addMemory(text: string): Promise<Memory> {
  return agentRequest("POST", "/memories", { text });
}

export function updateMemory(id: string, text: string): Promise<Memory> {
  return agentRequest("PUT", `/memories/${encodeURIComponent(id)}`, { text });
}

export function deleteMemory(id: string): Promise<void> {
  return agentRequest("DELETE", `/memories/${encodeURIComponent(id)}`);
}

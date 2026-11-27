import { useEffect, useState, type KeyboardEvent } from "react";
import type { Agent, AgentSnapshot, AgentState, WeatherSnapshot } from "../../types";
import PixelSprite from "./PixelSprite";
import { sceneFor, type Pose } from "./scene";
import { bed, outfits, palette, poses, precip, room, windowSky } from "./sprites";

interface Props {
  agent: AgentSnapshot | null;
  weather: WeatherSnapshot | null;
  settings: Pick<Agent, "textInputEnabled" | "quietStart" | "quietEnd">;
  onSend?: (text: string) => Promise<void>;
}

const MS_PER_CHAR = 60;
const MIN_TALK_MS = 2000;
const MAX_TALK_MS = 12000;
const SCENE_REFRESH_MS = 60 * 1000;
const FRAME_MS = 400;
const WANDER_MS = 4000;

const POSE_LABEL: Record<Pose, string> = {
  idle: "idle",
  think: "thinking",
  talk: "talking",
  sleep: "sleeping",
  confused: "confused",
};

function talkEndsAt(agent: AgentSnapshot | null): number {
  if (!agent || agent.state !== "talking") return 0;
  const duration = Math.min(MAX_TALK_MS, Math.max(MIN_TALK_MS, agent.reply.length * MS_PER_CHAR));
  return Date.parse(agent.at) + duration;
}

function width(grid: string[]): number {
  return grid[0]?.length ?? 0;
}

function pct(n: number, of: number): string {
  return `${(n / of) * 100}%`;
}

export default function AgentPanel({ agent, weather, settings, onSend }: Props) {
  const [, setTick] = useState(0);
  const [frame, setFrame] = useState(0);
  const [wander, setWander] = useState(0.5);
  const [inputOpen, setInputOpen] = useState(false);
  const [draft, setDraft] = useState("");

  const now = Date.now();
  const talkUntil = talkEndsAt(agent);
  const agentState: AgentState =
    agent?.state === "talking" ? (talkUntil > now ? "talking" : "idle") : (agent?.state ?? "idle");

  const scene = sceneFor({
    now: new Date(now),
    weather,
    agentState,
    quietStart: settings.quietStart,
    quietEnd: settings.quietEnd,
  });

  useEffect(() => {
    const id = setInterval(() => setTick((t) => t + 1), SCENE_REFRESH_MS);
    return () => clearInterval(id);
  }, []);

  useEffect(() => {
    const remaining = talkUntil - Date.now();
    if (remaining <= 0) return;
    const id = setTimeout(() => setTick((t) => t + 1), remaining);
    return () => clearTimeout(id);
  }, [talkUntil]);

  useEffect(() => {
    const id = setInterval(() => setFrame((f) => f + 1), FRAME_MS);
    return () => clearInterval(id);
  }, []);

  useEffect(() => {
    if (scene.pose !== "idle") return;
    const id = setInterval(() => setWander(Math.random()), WANDER_MS);
    return () => clearInterval(id);
  }, [scene.pose]);

  const send = () => {
    const text = draft.trim();
    if (!text) return;
    setInputOpen(false);
    setDraft("");
    onSend?.(text).catch(() => {});
  };

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter") send();
  };

  const roomW = width(room);
  const roomH = room.length;
  const frames = poses[scene.pose];
  const body = frames[frame % frames.length];
  const bodyW = width(body);
  const bodyH = body.length;
  const sky = windowSky[scene.sky];
  const bedX = roomW - width(bed) - 1;
  const bedY = roomH - bed.length;
  const sleeping = scene.pose === "sleep";
  const charX = sleeping ? bedX : Math.round(wander * (bedX - bodyW));
  const charY = sleeping ? bedY : roomH - bodyH;

  const bubble = scene.pose === "think" ? "…" : scene.pose === "talk" ? agent?.reply : null;

  const character = (
    <svg
      role="img"
      aria-label={`Assistant is ${POSE_LABEL[scene.pose]}`}
      viewBox={`0 0 ${bodyW} ${bodyH}`}
      className="agent-character-sprite"
    >
      <PixelSprite grid={body} palette={palette} />
      {!sleeping && <PixelSprite grid={outfits[scene.outfit]} palette={palette} />}
    </svg>
  );

  return (
    <section className="widget agent-panel" aria-label="Assistant">
      <div className="agent-room">
        <svg viewBox={`0 0 ${roomW} ${roomH}`} className="agent-room-sprite" aria-hidden="true">
          <PixelSprite grid={room} palette={palette} />
          <PixelSprite grid={sky} palette={palette} x={2} y={1} />
          <PixelSprite grid={precip[scene.precip]} palette={palette} x={2} y={1} />
          <PixelSprite grid={bed} palette={palette} x={bedX} y={bedY} />
        </svg>
        <div
          className="agent-character"
          style={{
            left: pct(charX, roomW),
            top: pct(charY, roomH),
            width: pct(bodyW, roomW),
            height: pct(bodyH, roomH),
          }}
        >
          {bubble && <div className="agent-bubble">{bubble}</div>}
          {settings.textInputEnabled ? (
            <button type="button" className="agent-talk-btn" aria-label="Talk to assistant" onClick={() => setInputOpen(true)}>
              {character}
            </button>
          ) : (
            character
          )}
        </div>
      </div>
      {settings.textInputEnabled && inputOpen && (
        <div className="agent-input">
          <input
            type="text"
            aria-label="Message"
            autoFocus
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={onKeyDown}
          />
          <button type="button" onClick={send}>
            Send
          </button>
        </div>
      )}
    </section>
  );
}

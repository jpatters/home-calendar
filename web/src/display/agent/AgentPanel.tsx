import { useEffect, useState, type KeyboardEvent } from "react";
import type { Agent, AgentSnapshot, AgentState, WeatherSnapshot } from "../../types";
import PixelSprite from "./PixelSprite";
import { sceneFor, type Pose } from "./scene";
import { CHAR_H, CHAR_W, SLEEP_VISIBLE_ROWS, composeCharacter, type Face } from "./sprites/character";
import {
  ROOM_H,
  ROOM_W,
  LOOKOUT_X,
  SLEEP_X,
  SLEEP_Y,
  STAND_Y,
  WANDER_MAX_X,
  WANDER_MIN_X,
  roomRects,
} from "./sprites/room";

interface Props {
  agent: AgentSnapshot | null;
  weather: WeatherSnapshot | null;
  settings: Pick<Agent, "textInputEnabled" | "quietStart" | "quietEnd" | "voiceDelayMs">;
  onSend?: (text: string) => Promise<void>;
}

const MS_PER_CHAR = 60;
const MIN_TALK_MS = 2000;
const MAX_TALK_MS = 12000;
const SCENE_REFRESH_MS = 60 * 1000;
const FRAME_MS = 200;
const WANDER_MS = 4000;
const SEND_FAILED_REPLY = "I couldn't reach the server. Try again.";

type Speech = Pick<AgentSnapshot, "state" | "reply" | "at" | "spoken">;

const POSE_LABEL: Record<Pose, string> = {
  idle: "idle",
  think: "thinking",
  lookout: "looking out the window",
  talk: "talking",
  sleep: "sleeping",
  confused: "confused",
};

// Talking and apologizing after an error both "speak" the reply: the bubble
// stays up long enough to read it, then the assistant goes back to idle.
function speaks(state: AgentState | undefined): boolean {
  return state === "talking" || state === "error";
}

// A spoken reply starts once the voice device has had time to start playing
// it; a typed one starts straight away.
function speechStartsAt(agent: Speech | null, voiceDelayMs: number): number {
  if (!agent || !speaks(agent.state)) return 0;
  return Date.parse(agent.at) + (agent.spoken ? voiceDelayMs : 0);
}

function speechEndsAt(agent: Speech | null, voiceDelayMs: number): number {
  if (!agent || !speaks(agent.state)) return 0;
  const duration = Math.min(MAX_TALK_MS, Math.max(MIN_TALK_MS, agent.reply.length * MS_PER_CHAR));
  return speechStartsAt(agent, voiceDelayMs) + duration;
}

function faceFor(pose: Pose, frame: number): Face {
  switch (pose) {
    case "talk":
      return frame % 2 === 0 ? "talk" : "idle";
    case "think":
      return "think";
    case "lookout":
      return "back";
    case "confused":
      return "confused";
    case "sleep":
      return "sleep";
    default:
      return frame % 18 >= 16 ? "blink" : "idle";
  }
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
  const [sendFailure, setSendFailure] = useState<Speech | null>(null);
  const [atWindow, setAtWindow] = useState(false);

  const now = Date.now();
  const speech: Speech | null =
    sendFailure && (!agent || Date.parse(sendFailure.at) >= Date.parse(agent.at)) ? sendFailure : agent;
  const talkFrom = speechStartsAt(speech, settings.voiceDelayMs);
  const talkUntil = speechEndsAt(speech, settings.voiceDelayMs);
  const reported = speech?.state ?? "idle";
  const agentState: AgentState = !speaks(reported)
    ? reported
    : now >= talkUntil
      ? "idle"
      : now < talkFrom
        ? "thinking"
        : reported;

  const scene = sceneFor({
    now: new Date(now),
    weather,
    agentState,
    tool: agent?.tool,
    quietStart: settings.quietStart,
    quietEnd: settings.quietEnd,
  });

  // After looking out the window Ziggy stays there to give the answer, and
  // leaves once the turn is over.
  if (scene.pose === "lookout" && !atWindow) setAtWindow(true);
  if (atWindow && (scene.pose === "idle" || scene.pose === "sleep")) setAtWindow(false);

  useEffect(() => {
    const id = setInterval(() => setTick((t) => t + 1), SCENE_REFRESH_MS);
    return () => clearInterval(id);
  }, []);

  useEffect(() => {
    const timers = [talkFrom, talkUntil]
      .map((at) => at - Date.now())
      .filter((remaining) => remaining > 0)
      .map((remaining) => setTimeout(() => setTick((t) => t + 1), remaining));
    return () => timers.forEach(clearTimeout);
  }, [talkFrom, talkUntil]);

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
    onSend?.(text).catch(() => {
      setSendFailure({ state: "error", reply: SEND_FAILED_REPLY, at: new Date().toISOString() });
    });
  };

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter") send();
  };

  const sleeping = scene.pose === "sleep";
  const sprite = composeCharacter(scene.outfit, faceFor(scene.pose, frame));
  const grid = sleeping ? sprite.grid.slice(0, SLEEP_VISIBLE_ROWS) : sprite.grid;
  const charX = sleeping
    ? SLEEP_X
    : atWindow || scene.pose === "lookout"
      ? LOOKOUT_X
      : Math.round(WANDER_MIN_X + wander * (WANDER_MAX_X - WANDER_MIN_X));
  const charY = sleeping ? SLEEP_Y : STAND_Y;
  const charH = sleeping ? SLEEP_VISIBLE_ROWS : CHAR_H;
  const rects = roomRects({
    sky: scene.sky,
    conditions: scene.conditions,
    tick: frame,
    blanket: scene.outfit === "christmas" ? "#2e8b57" : "#5b7fd6",
  });

  const bubble =
    scene.pose === "think" ? "…" : scene.pose === "talk" || scene.pose === "confused" ? speech?.reply : null;

  const character = (
    <svg
      role="img"
      aria-label={`Assistant is ${POSE_LABEL[scene.pose]}`}
      viewBox={`0 0 ${CHAR_W} ${charH}`}
      className="agent-character-sprite"
    >
      <PixelSprite grid={grid} palette={sprite.palette} />
    </svg>
  );

  return (
    <section className="widget agent-panel" aria-label="Assistant">
      <div className="agent-room">
        <svg viewBox={`0 0 ${ROOM_W} ${ROOM_H}`} className="agent-room-sprite" aria-hidden="true">
          <g shapeRendering="crispEdges">
            {rects.map((r, i) => (
              <rect key={i} x={r.x} y={r.y} width={r.w} height={r.h} fill={r.fill} opacity={r.opacity} />
            ))}
          </g>
        </svg>
        {bubble && <div className="agent-bubble">{bubble}</div>}
        <div
          className={`agent-character${scene.pose === "lookout" ? " to-window" : ""}`}
          style={{
            left: pct(charX, ROOM_W),
            top: pct(charY, ROOM_H),
            width: pct(CHAR_W, ROOM_W),
            height: pct(charH, ROOM_H),
          }}
        >
          {scene.pose === "confused" && <div className="agent-mark" aria-hidden="true">?</div>}
          {sleeping && <div className="agent-zzz" aria-hidden="true">z z z</div>}
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

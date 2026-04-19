import { useEffect, useRef, useState } from "react";
import type { CalendarEvent, Config, WSFrame, WeatherSnapshot } from "./types";

export interface LiveData {
  ready: boolean;
  connected: boolean;
  config: Config | null;
  events: CalendarEvent[];
  weather: WeatherSnapshot | null;
}

const INITIAL: LiveData = {
  ready: false,
  connected: false,
  config: null,
  events: [],
  weather: null,
};

export function useLiveData(): LiveData {
  const [state, setState] = useState<LiveData>(INITIAL);
  const wsRef = useRef<WebSocket | null>(null);
  const backoffRef = useRef(1000);

  useEffect(() => {
    let closed = false;
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null;

    const connect = () => {
      if (closed) return;
      const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
      const url = `${proto}//${window.location.host}/api/ws`;
      const ws = new WebSocket(url);
      wsRef.current = ws;

      ws.onopen = () => {
        backoffRef.current = 1000;
        setState((s) => ({ ...s, connected: true }));
      };

      ws.onmessage = (ev) => {
        let frame: WSFrame;
        try {
          frame = JSON.parse(ev.data);
        } catch {
          return;
        }
        setState((s) => {
          switch (frame.type) {
            case "snapshot":
              return {
                ready: true,
                connected: true,
                config: frame.config,
                events: frame.events ?? [],
                weather: frame.weather,
              };
            case "calendar":
              return { ...s, events: frame.events ?? [] };
            case "weather":
              return { ...s, weather: frame.weather };
            case "config":
              return { ...s, config: frame.config };
            default:
              return s;
          }
        });
      };

      ws.onclose = () => {
        setState((s) => ({ ...s, connected: false }));
        if (closed) return;
        const delay = backoffRef.current;
        backoffRef.current = Math.min(delay * 2, 30000);
        reconnectTimer = setTimeout(connect, delay);
      };

      ws.onerror = () => {
        ws.close();
      };
    };

    connect();

    return () => {
      closed = true;
      if (reconnectTimer) clearTimeout(reconnectTimer);
      wsRef.current?.close();
    };
  }, []);

  return state;
}

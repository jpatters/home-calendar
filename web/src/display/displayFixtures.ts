import type { LiveData } from "../useLiveData";
import type { Config } from "../types";

export function buildConfig(
  overrides: Partial<{
    weatherEnabled: boolean;
    tideEnabled: boolean;
    snowDayEnabled: boolean;
    baseballEnabled: boolean;
    hockeyEnabled: boolean;
    poolEnabled: boolean;
    hotTubEnabled: boolean;
    calendarEnabled: boolean;
    clockEnabled: boolean;
    agentEnabled: boolean;
    agentTextInputEnabled: boolean;
    layout: unknown;
  }> = {},
): Config {
  return {
    calendars: [],
    weather: {
      latitude: 0,
      longitude: 0,
      units: "metric",
      timezone: "UTC",
      location: "Test",
      enabled: overrides.weatherEnabled ?? true,
    },
    tide: {
      stationCode: "01710",
      units: "metric",
      location: "Test",
      enabled: overrides.tideEnabled ?? true,
    },
    snowDay: {
      url: "https://example.com",
      enabled: overrides.snowDayEnabled ?? true,
    },
    baseball: {
      enabled: overrides.baseballEnabled ?? true,
      teamId: 0,
      teamName: "",
      teamAbbr: "",
    },
    hockey: {
      enabled: overrides.hockeyEnabled ?? true,
      teamAbbr: "",
      teamName: "",
    },
    pool: {
      enabled: overrides.poolEnabled ?? true,
      deviceUrl: "http://pool.test",
    },
    hotTub: {
      enabled: overrides.hotTubEnabled ?? true,
      host: "192.168.1.50",
    },
    agent: {
      enabled: overrides.agentEnabled ?? false,
      model: "gpt-6.1-terra",
      effort: "low",
      textInputEnabled: overrides.agentTextInputEnabled ?? false,
      quietStart: "22:00",
      quietEnd: "07:00",
    },
    display: {
      defaultView: "week",
      calendarRefreshSeconds: 300,
      weatherRefreshSeconds: 900,
      tideRefreshSeconds: 3600,
      baseballRefreshSeconds: 600,
      hockeyRefreshSeconds: 600,
      poolRefreshSeconds: 30,
      hotTubRefreshSeconds: 30,
      theme: "default",
      mode: "light",
      calendarEnabled: overrides.calendarEnabled ?? true,
      clockEnabled: overrides.clockEnabled ?? true,
    },
    layout: overrides.layout,
  };
}

export function buildLive(config: Config): LiveData {
  return {
    ready: true,
    connected: true,
    config,
    events: [],
    weather: null,
    snowday: null,
    tide: null,
    baseball: null,
    hockey: null,
    pool: null,
    hottub: null,
    agent: null,
  };
}

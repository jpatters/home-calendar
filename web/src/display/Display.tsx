import { useState } from "react";
import type { CalendarEvent } from "../types";
import type { LiveData } from "../useLiveData";
import CalendarView from "./CalendarView";
import ClockWidget from "./ClockWidget";
import WeatherWidget from "./WeatherWidget";
import SnowDayWidget from "./SnowDayWidget";
import TideWidget from "./TideWidget";
import TideModal from "./TideModal";
import BaseballWidget from "./BaseballWidget";
import EventModal from "./EventModal";
import DayModal from "./DayModal";

interface Props {
  live: LiveData;
}

export default function Display({ live }: Props) {
  const [selectedEvent, setSelectedEvent] = useState<CalendarEvent | null>(null);
  const [selectedDay, setSelectedDay] = useState<string | null>(null);
  const [tideOpen, setTideOpen] = useState(false);

  if (!live.ready) {
    return <div className="loading">Loading…</div>;
  }

  const events = live.events;
  const display = live.config?.display;
  const calendarEnabled = display?.calendarEnabled ?? true;
  const clockEnabled = display?.clockEnabled ?? true;
  const weatherEnabled = live.config?.weather.enabled ?? true;
  const snowDayEnabled = live.config?.snowDay.enabled ?? true;
  const tideEnabled = live.config?.tide.enabled ?? true;
  const baseballEnabled = live.config?.baseball.enabled ?? true;

  return (
    <div
      className="display-grid"
      data-calendar-hidden={!calendarEnabled ? "true" : undefined}
    >
      {calendarEnabled && (
        <main className="calendar-pane">
          <CalendarView
            events={events}
            defaultView={display?.defaultView ?? "week"}
            onEventClick={setSelectedEvent}
            onDayClick={setSelectedDay}
          />
        </main>
      )}

      <aside className="widget-pane">
        {clockEnabled && <ClockWidget />}
        {weatherEnabled && (
          <WeatherWidget weather={live.weather} config={live.config?.weather} />
        )}
        {snowDayEnabled && (
          <SnowDayWidget snowday={live.snowday} config={live.config?.snowDay} />
        )}
        {tideEnabled && (
          <TideWidget
            tide={live.tide}
            config={live.config?.tide}
            onOpen={() => setTideOpen(true)}
          />
        )}
        {baseballEnabled && (
          <BaseballWidget baseball={live.baseball} config={live.config?.baseball} />
        )}
        <div className="connection-indicator">
          <span className={live.connected ? "dot ok" : "dot bad"} />
          {live.connected ? "live" : "reconnecting…"}
        </div>
      </aside>

      {tideOpen && live.tide && (
        <TideModal
          tide={live.tide}
          config={live.config?.tide}
          onClose={() => setTideOpen(false)}
        />
      )}
      {selectedEvent && (
        <EventModal event={selectedEvent} onClose={() => setSelectedEvent(null)} />
      )}
      {selectedDay && (
        <DayModal
          dayISO={selectedDay}
          events={events}
          onClose={() => setSelectedDay(null)}
          onEventClick={(ev) => {
            setSelectedDay(null);
            setSelectedEvent(ev);
          }}
        />
      )}
    </div>
  );
}

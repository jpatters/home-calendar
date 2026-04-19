import { useState } from "react";
import type { CalendarEvent } from "../types";
import type { LiveData } from "../useLiveData";
import CalendarView from "./CalendarView";
import ClockWidget from "./ClockWidget";
import WeatherWidget from "./WeatherWidget";
import SnowDayWidget from "./SnowDayWidget";
import EventModal from "./EventModal";
import DayModal from "./DayModal";

interface Props {
  live: LiveData;
}

export default function Display({ live }: Props) {
  const [selectedEvent, setSelectedEvent] = useState<CalendarEvent | null>(null);
  const [selectedDay, setSelectedDay] = useState<string | null>(null);

  if (!live.ready) {
    return <div className="loading">Loading…</div>;
  }

  const events = live.events;
  const display = live.config?.display;

  return (
    <div className="display-grid">
      <main className="calendar-pane">
        <CalendarView
          events={events}
          defaultView={display?.defaultView ?? "week"}
          onEventClick={setSelectedEvent}
          onDayClick={setSelectedDay}
        />
      </main>

      <aside className="widget-pane">
        <ClockWidget />
        <WeatherWidget weather={live.weather} config={live.config?.weather} />
        <SnowDayWidget snowday={live.snowday} config={live.config?.snowDay} />
        <div className="connection-indicator">
          <span className={live.connected ? "dot ok" : "dot bad"} />
          {live.connected ? "live" : "reconnecting…"}
        </div>
      </aside>

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

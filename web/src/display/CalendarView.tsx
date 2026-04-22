import FullCalendar from "@fullcalendar/react";
import dayGridPlugin from "@fullcalendar/daygrid";
import timeGridPlugin from "@fullcalendar/timegrid";
import interactionPlugin, { type DateClickArg } from "@fullcalendar/interaction";
import type { DatesSetArg, DayHeaderContentArg, EventClickArg } from "@fullcalendar/core";
import { useMemo, useRef, useState } from "react";
import { refreshCalendars } from "../api";
import type { CalendarEvent, Weather, WeatherDaily, WeatherSnapshot } from "../types";
import { labelForCode, WeatherIcon } from "./weatherIcons";

interface Props {
  events: CalendarEvent[];
  defaultView: "day" | "week" | "month";
  onEventClick: (ev: CalendarEvent) => void;
  onDayClick: (dayISO: string) => void;
  weather?: WeatherSnapshot | null;
  weatherConfig?: Weather;
}

const VIEW_MAP = {
  day: "timeGridDay",
  week: "timeGridWeek",
  month: "dayGridMonth",
} as const;

function tempUnit(units: string | undefined): string {
  return units === "imperial" ? "°F" : "°C";
}

function localDateKey(d: Date): string {
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

export default function CalendarView({
  events,
  defaultView,
  onEventClick,
  onDayClick,
  weather,
  weatherConfig,
}: Props) {
  const ref = useRef<FullCalendar | null>(null);
  const [title, setTitle] = useState("");

  const fcEvents = events.map((e) => ({
    id: e.id,
    title: e.title,
    start: e.start,
    end: e.end,
    allDay: e.allDay,
    backgroundColor: e.calendarColor,
    borderColor: e.calendarColor,
    extendedProps: { source: e } satisfies { source: CalendarEvent },
  }));

  const handleEventClick = (arg: EventClickArg) => {
    const source = arg.event.extendedProps.source as CalendarEvent | undefined;
    if (source) onEventClick(source);
  };

  const handleDateClick = (arg: DateClickArg) => {
    onDayClick(arg.dateStr);
  };

  const setView = (key: "day" | "week" | "month") => {
    ref.current?.getApi().changeView(VIEW_MAP[key]);
  };

  const handleDatesSet = (arg: DatesSetArg) => {
    setTitle(arg.view.title);
  };

  const weatherEnabled = weatherConfig?.enabled !== false;
  const dailyByDate = useMemo(() => {
    const map = new Map<string, WeatherDaily>();
    if (weather && weatherEnabled) {
      for (const d of weather.daily) map.set(d.date, d);
    }
    return map;
  }, [weather, weatherEnabled]);

  const units = weatherConfig?.units ?? weather?.units;

  const renderDayHeader = (arg: DayHeaderContentArg) => {
    const viewType = arg.view.type;
    if (viewType !== "timeGridWeek" && viewType !== "timeGridDay") {
      return true;
    }
    // Assumes the browser's local timezone matches weather.timezone.
    // True for a wall display configured for its own location; may drift
    // by a day otherwise.
    const key = localDateKey(arg.date);
    const day = dailyByDate.get(key);
    if (!day) return true;
    const high = `${Math.round(day.maxC)}${tempUnit(units)}`;
    const label = `${labelForCode(day.weatherCode)}, high ${high}`;
    return (
      <span className="fc-custom-day-header">
        {arg.text}
        <span className="calendar-day-weather" aria-label={label}>
          <WeatherIcon code={day.weatherCode} className="calendar-day-icon" />
          <span className="calendar-day-high">{high}</span>
        </span>
      </span>
    );
  };

  return (
    <div className="calendar-wrapper">
      <div className="calendar-toolbar">
        <div className="group">
          <button onClick={() => ref.current?.getApi().prev()}>‹</button>
          <button onClick={() => ref.current?.getApi().today()}>Today</button>
          <button onClick={() => ref.current?.getApi().next()}>›</button>
        </div>
        <h2 className="calendar-title">{title}</h2>
        <div className="group view-switch">
          <button onClick={() => setView("day")}>Day</button>
          <button onClick={() => setView("week")}>Week</button>
          <button onClick={() => setView("month")}>Month</button>
        </div>
        <div className="group">
          <button onClick={() => void refreshCalendars()}>↻</button>
        </div>
      </div>
      <FullCalendar
        ref={ref}
        plugins={[dayGridPlugin, timeGridPlugin, interactionPlugin]}
        initialView={VIEW_MAP[defaultView]}
        headerToolbar={false}
        height="100%"
        events={fcEvents}
        eventClick={handleEventClick}
        dateClick={handleDateClick}
        datesSet={handleDatesSet}
        dayHeaderContent={renderDayHeader}
        nowIndicator
        longPressDelay={250}
        selectLongPressDelay={250}
        eventLongPressDelay={250}
      />
    </div>
  );
}

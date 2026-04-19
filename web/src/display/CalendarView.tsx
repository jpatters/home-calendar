import FullCalendar from "@fullcalendar/react";
import dayGridPlugin from "@fullcalendar/daygrid";
import timeGridPlugin from "@fullcalendar/timegrid";
import interactionPlugin, { type DateClickArg } from "@fullcalendar/interaction";
import type { DatesSetArg, EventClickArg } from "@fullcalendar/core";
import { useRef, useState } from "react";
import { refreshCalendars } from "../api";
import type { CalendarEvent } from "../types";

interface Props {
  events: CalendarEvent[];
  defaultView: "day" | "week" | "month";
  onEventClick: (ev: CalendarEvent) => void;
  onDayClick: (dayISO: string) => void;
}

const VIEW_MAP = {
  day: "timeGridDay",
  week: "timeGridWeek",
  month: "dayGridMonth",
} as const;

export default function CalendarView({ events, defaultView, onEventClick, onDayClick }: Props) {
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
        nowIndicator
        longPressDelay={250}
        selectLongPressDelay={250}
        eventLongPressDelay={250}
      />
    </div>
  );
}

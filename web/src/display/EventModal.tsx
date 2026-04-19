import type { CalendarEvent } from "../types";

interface Props {
  event: CalendarEvent;
  onClose: () => void;
}

// All-day event dates arrive as "YYYY-MM-DD". Parse them as local midnight
// so the browser doesn't shift the date across timezones.
function parseEventDate(iso: string, allDay: boolean): Date {
  return allDay ? new Date(iso + "T00:00:00") : new Date(iso);
}

function fmtRange(ev: CalendarEvent): string {
  const start = parseEventDate(ev.start, ev.allDay);
  const end = parseEventDate(ev.end, ev.allDay);
  if (ev.allDay) {
    const sameDay = start.toDateString() === new Date(end.getTime() - 1).toDateString();
    if (sameDay) {
      return start.toLocaleDateString([], { weekday: "long", month: "long", day: "numeric" });
    }
    return `${start.toLocaleDateString()} – ${end.toLocaleDateString()}`;
  }
  const sameDay = start.toDateString() === end.toDateString();
  const startStr = start.toLocaleString([], { weekday: "long", month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });
  const endStr = sameDay
    ? end.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
    : end.toLocaleString([], { weekday: "short", month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });
  return `${startStr} – ${endStr}`;
}

export default function EventModal({ event, onClose }: Props) {
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal-header" style={{ borderColor: event.calendarColor }}>
          <div className="swatch" style={{ background: event.calendarColor }} />
          <h2>{event.title || "(no title)"}</h2>
          <button className="close-btn" onClick={onClose}>×</button>
        </div>
        <div className="modal-body">
          <div className="event-when">{fmtRange(event)}</div>
          <div className="event-cal">{event.calendarName}</div>
          {event.location && <div className="event-location">📍 {event.location}</div>}
          {event.description && <div className="event-desc">{event.description}</div>}
        </div>
      </div>
    </div>
  );
}

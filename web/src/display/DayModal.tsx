import type { CalendarEvent } from "../types";

interface Props {
  dayISO: string;
  events: CalendarEvent[];
  onClose: () => void;
  onEventClick: (ev: CalendarEvent) => void;
}

// All-day event dates arrive as "YYYY-MM-DD". Parse them as local midnight
// so the browser doesn't shift the date across timezones.
function parseEventDate(iso: string, allDay: boolean): Date {
  return allDay ? new Date(iso + "T00:00:00") : new Date(iso);
}

function sameDay(ev: CalendarEvent, bISO: string): boolean {
  const a = parseEventDate(ev.start, ev.allDay);
  const b = new Date(bISO + "T00:00:00");
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
}

function overlapsDay(ev: CalendarEvent, dayISO: string): boolean {
  const dayStart = new Date(dayISO + "T00:00:00");
  const dayEnd = new Date(dayStart.getTime() + 24 * 60 * 60 * 1000);
  const start = parseEventDate(ev.start, ev.allDay);
  const end = parseEventDate(ev.end, ev.allDay);
  return start < dayEnd && end > dayStart;
}

export default function DayModal({ dayISO, events, onClose, onEventClick }: Props) {
  const dayEvents = events
    .filter((e) => overlapsDay(e, dayISO) || sameDay(e, dayISO))
    .sort((a, b) => parseEventDate(a.start, a.allDay).getTime() - parseEventDate(b.start, b.allDay).getTime());

  const dateLabel = new Date(dayISO + "T00:00:00").toLocaleDateString([], {
    weekday: "long",
    month: "long",
    day: "numeric",
    year: "numeric",
  });

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal day-modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal-header">
          <h2>{dateLabel}</h2>
          <button className="close-btn" onClick={onClose}>×</button>
        </div>
        <div className="modal-body">
          {dayEvents.length === 0 ? (
            <div className="empty">No events</div>
          ) : (
            <ul className="event-list">
              {dayEvents.map((e) => (
                <li key={e.id} className="event-row" onClick={() => onEventClick(e)}>
                  <div className="swatch" style={{ background: e.calendarColor }} />
                  <div className="event-row-time">
                    {e.allDay
                      ? "All day"
                      : new Date(e.start).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}
                  </div>
                  <div className="event-row-title">{e.title || "(no title)"}</div>
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>
    </div>
  );
}

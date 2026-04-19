import type { CalendarEvent } from "../types";

interface Props {
  dayISO: string;
  events: CalendarEvent[];
  onClose: () => void;
  onEventClick: (ev: CalendarEvent) => void;
}

function sameDay(aISO: string, bISO: string): boolean {
  const a = new Date(aISO);
  const b = new Date(bISO);
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
}

function overlapsDay(ev: CalendarEvent, dayISO: string): boolean {
  const dayStart = new Date(dayISO + "T00:00:00");
  const dayEnd = new Date(dayStart.getTime() + 24 * 60 * 60 * 1000);
  const start = new Date(ev.start);
  const end = new Date(ev.end);
  return start < dayEnd && end > dayStart;
}

export default function DayModal({ dayISO, events, onClose, onEventClick }: Props) {
  const dayEvents = events
    .filter((e) => overlapsDay(e, dayISO) || sameDay(e.start, dayISO))
    .sort((a, b) => new Date(a.start).getTime() - new Date(b.start).getTime());

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

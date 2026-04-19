import type { Calendar } from "../types";

interface Props {
  value: Calendar[];
  onChange: (c: Calendar[]) => void;
}

const DEFAULT_COLORS = ["#4285f4", "#0b8043", "#d50000", "#f4511e", "#8e24aa", "#3f51b5"];

function randomId(): string {
  if (typeof crypto !== "undefined" && crypto.randomUUID) return crypto.randomUUID();
  return Math.random().toString(36).slice(2);
}

export default function CalendarsPanel({ value, onChange }: Props) {
  const update = (i: number, patch: Partial<Calendar>) => {
    const next = value.slice();
    next[i] = { ...next[i], ...patch };
    onChange(next);
  };
  const remove = (i: number) => {
    const next = value.slice();
    next.splice(i, 1);
    onChange(next);
  };
  const add = () => {
    onChange([
      ...value,
      {
        id: randomId(),
        name: "New calendar",
        color: DEFAULT_COLORS[value.length % DEFAULT_COLORS.length],
        url: "",
      },
    ]);
  };

  return (
    <div className="panel">
      <h2>Calendars</h2>
      <p className="hint">
        Paste the "Secret address in iCal format" URL from each Google Calendar's settings. Each calendar's events are merged into the display.
      </p>
      <div className="calendar-list">
        {value.map((cal, i) => (
          <div className="calendar-row" key={cal.id}>
            <input
              className="color"
              type="color"
              value={cal.color}
              onChange={(e) => update(i, { color: e.target.value })}
            />
            <input
              className="name"
              type="text"
              placeholder="Name"
              value={cal.name}
              onChange={(e) => update(i, { name: e.target.value })}
            />
            <input
              className="url"
              type="url"
              placeholder="https://calendar.google.com/calendar/ical/.../basic.ics"
              value={cal.url}
              onChange={(e) => update(i, { url: e.target.value })}
            />
            <button className="danger" onClick={() => remove(i)}>Remove</button>
          </div>
        ))}
      </div>
      <button onClick={add}>+ Add calendar</button>
    </div>
  );
}

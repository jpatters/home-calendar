export function formatGameTime(iso: string, now: Date = new Date()): string {
  const date = new Date(iso);
  const time = date.toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
  const dayDiff = daysBetween(now, date);
  if (dayDiff === 0) return `Today · ${time}`;
  if (dayDiff === 1) return `Tomorrow · ${time}`;
  const datePart = date.toLocaleDateString([], {
    weekday: "short",
    month: "short",
    day: "numeric",
  });
  return `${datePart} · ${time}`;
}

function daysBetween(a: Date, b: Date): number {
  const startA = Date.UTC(a.getFullYear(), a.getMonth(), a.getDate());
  const startB = Date.UTC(b.getFullYear(), b.getMonth(), b.getDate());
  return Math.round((startB - startA) / (24 * 60 * 60 * 1000));
}

export function formatInning(half: string | undefined, inning: number | undefined): string {
  if (!inning || inning <= 0) return "";
  const ordinal = inningOrdinal(inning);
  switch (half) {
    case "top":
      return `Top ${ordinal}`;
    case "bottom":
      return `Bot ${ordinal}`;
    case "middle":
      return `Mid ${ordinal}`;
    case "end":
      return `End ${ordinal}`;
    default:
      return ordinal;
  }
}

function inningOrdinal(n: number): string {
  const mod100 = n % 100;
  if (mod100 >= 11 && mod100 <= 13) return `${n}th`;
  switch (n % 10) {
    case 1:
      return `${n}st`;
    case 2:
      return `${n}nd`;
    case 3:
      return `${n}rd`;
    default:
      return `${n}th`;
  }
}

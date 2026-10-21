import { ordinal } from "./gameFormat";

export function formatInning(half: string | undefined, inning: number | undefined): string {
  if (!inning || inning <= 0) return "";
  const label = ordinal(inning);
  switch (half) {
    case "top":
      return `Top ${label}`;
    case "bottom":
      return `Bot ${label}`;
    case "middle":
      return `Mid ${label}`;
    case "end":
      return `End ${label}`;
    default:
      return label;
  }
}

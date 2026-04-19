const METRES_TO_FEET = 3.281;

export function formatHeight(meters: number, units: string | undefined): string {
  if (units === "imperial") {
    return `${(meters * METRES_TO_FEET).toFixed(1)} ft`;
  }
  return `${meters.toFixed(1)} m`;
}

export function formatTime(iso: string): string {
  return new Date(iso).toLocaleTimeString([], {
    hour: "numeric",
    minute: "2-digit",
  });
}

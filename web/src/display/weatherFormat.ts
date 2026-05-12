export function tempUnit(units: string | undefined): string {
  return units === "imperial" ? "°F" : "°C";
}

export function speedUnit(units: string | undefined): string {
  return units === "imperial" ? "mph" : "km/h";
}

export function precipUnit(units: string | undefined): string {
  return units === "imperial" ? "in" : "mm";
}

const compassPoints = [
  "N", "NNE", "NE", "ENE", "E", "ESE", "SE", "SSE",
  "S", "SSW", "SW", "WSW", "W", "WNW", "NW", "NNW",
];

export function compassFromDegrees(deg: number): string {
  const idx = Math.round(((deg % 360) + 360) % 360 / 22.5) % 16;
  return compassPoints[idx];
}

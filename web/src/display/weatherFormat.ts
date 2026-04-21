export function tempUnit(units: string | undefined): string {
  return units === "imperial" ? "°F" : "°C";
}

export function speedUnit(units: string | undefined): string {
  return units === "imperial" ? "mph" : "km/h";
}

export function precipUnit(units: string | undefined): string {
  return units === "imperial" ? "in" : "mm";
}

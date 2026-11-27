export function formatTemp(celsius: number): string {
  return `${celsius.toFixed(1)}°C`;
}

const STEP_C = 0.5;

// Moves one half-degree step in the direction of delta, landing on the
// half-degree grid even when the heater reports a target between steps.
export function stepTarget(current: number, delta: 1 | -1): number {
  const steps = current / STEP_C;
  const next = delta > 0 ? Math.floor(steps + 1e-9) + 1 : Math.ceil(steps - 1e-9) - 1;
  return next * STEP_C;
}

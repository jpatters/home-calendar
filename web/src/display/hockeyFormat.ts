import { ordinal } from "./gameFormat";

const REGULATION_PERIODS = 3;

// formatPeriod labels an NHL period: "1st".."3rd" in regulation, "OT" for the
// first overtime, "2OT"/"3OT" for playoff multi-overtime, "SO" for a shootout.
export function formatPeriod(period: number | undefined, periodType: string | undefined): string {
  if (!period || period <= 0) return "";
  switch (periodType) {
    case "SO":
      return "SO";
    case "OT": {
      const n = period - REGULATION_PERIODS;
      return n <= 1 ? "OT" : `${n}OT`;
    }
    default:
      return ordinal(period);
  }
}

// formatFinal labels a completed game: "Final", or "Final/OT" and "Final/SO"
// for games decided past regulation.
export function formatFinal(periodType: string | undefined): string {
  if (periodType === "OT" || periodType === "SO") return `Final/${periodType}`;
  return "Final";
}

import { describe, expect, test } from "vitest";
import { formatFinal, formatPeriod } from "./hockeyFormat";

describe("formatPeriod", () => {
  test("regulation periods use ordinals", () => {
    expect(formatPeriod(1, "REG")).toBe("1st");
    expect(formatPeriod(2, "REG")).toBe("2nd");
    expect(formatPeriod(3, "REG")).toBe("3rd");
  });

  test("first overtime is OT", () => {
    expect(formatPeriod(4, "OT")).toBe("OT");
  });

  test("later overtimes are numbered", () => {
    expect(formatPeriod(5, "OT")).toBe("2OT");
    expect(formatPeriod(6, "OT")).toBe("3OT");
  });

  test("shootout is SO", () => {
    expect(formatPeriod(5, "SO")).toBe("SO");
  });

  test("missing period gives empty string", () => {
    expect(formatPeriod(undefined, undefined)).toBe("");
    expect(formatPeriod(0, "REG")).toBe("");
  });
});

describe("formatFinal", () => {
  test("regulation finals are plain Final", () => {
    expect(formatFinal("REG")).toBe("Final");
    expect(formatFinal(undefined)).toBe("Final");
  });

  test("overtime and shootout finals are suffixed", () => {
    expect(formatFinal("OT")).toBe("Final/OT");
    expect(formatFinal("SO")).toBe("Final/SO");
  });
});

import { afterEach, describe, expect, test } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import WeatherPanel from "./WeatherPanel";
import type { Weather } from "../types";

function baseConfig(): Weather {
  return {
    enabled: true,
    latitude: 43.65,
    longitude: -79.38,
    units: "metric",
    timezone: "auto",
    location: "Toronto",
    ecowittUrl: "",
  };
}

afterEach(() => {
  cleanup();
});

describe("WeatherPanel ecowitt url", () => {
  test("renders an input for the ecowitt url", () => {
    render(<WeatherPanel value={baseConfig()} onChange={() => {}} />);
    expect(screen.getByLabelText(/ecowitt/i)).not.toBeNull();
  });

  test("typing in the ecowitt input emits onChange with the new url", () => {
    let captured: Weather | null = null;
    render(
      <WeatherPanel
        value={baseConfig()}
        onChange={(w) => {
          captured = w;
        }}
      />,
    );
    const input = screen.getByLabelText(/ecowitt/i) as HTMLInputElement;
    fireEvent.change(input, { target: { value: "http://192.168.1.10/get_livedata_info" } });
    expect(captured).not.toBeNull();
    expect(captured!.ecowittUrl).toBe("http://192.168.1.10/get_livedata_info");
  });

});

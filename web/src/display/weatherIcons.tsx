import type { IconType } from "react-icons";
import {
  BsCloudDrizzleFill,
  BsCloudFogFill,
  BsCloudLightningFill,
  BsCloudLightningRainFill,
  BsCloudRainFill,
  BsCloudRainHeavyFill,
  BsCloudsFill,
  BsCloudSlashFill,
  BsCloudSnowFill,
  BsCloudSunFill,
  BsSnow3,
  BsSunFill,
} from "react-icons/bs";

const CODE_TO_ICON: Record<number, IconType> = {
  0: BsSunFill,
  1: BsCloudSunFill,
  2: BsCloudSunFill,
  3: BsCloudsFill,
  45: BsCloudFogFill,
  48: BsCloudFogFill,
  51: BsCloudDrizzleFill,
  53: BsCloudDrizzleFill,
  55: BsCloudRainFill,
  61: BsCloudRainFill,
  63: BsCloudRainFill,
  65: BsCloudRainHeavyFill,
  71: BsCloudSnowFill,
  73: BsCloudSnowFill,
  75: BsSnow3,
  80: BsCloudRainFill,
  81: BsCloudRainHeavyFill,
  82: BsCloudLightningRainFill,
  95: BsCloudLightningFill,
  96: BsCloudLightningRainFill,
  99: BsCloudLightningRainFill,
};

const CODE_TO_LABEL: Record<number, string> = {
  0: "Clear",
  1: "Mainly clear",
  2: "Partly cloudy",
  3: "Overcast",
  45: "Fog",
  48: "Rime fog",
  51: "Light drizzle",
  53: "Drizzle",
  55: "Heavy drizzle",
  61: "Light rain",
  63: "Rain",
  65: "Heavy rain",
  71: "Light snow",
  73: "Snow",
  75: "Heavy snow",
  80: "Showers",
  81: "Showers",
  82: "Violent showers",
  95: "Thunderstorm",
  96: "T-storm w/ hail",
  99: "T-storm w/ hail",
};

export function labelForCode(code: number): string {
  return CODE_TO_LABEL[code] ?? "—";
}

interface WeatherIconProps {
  code: number;
  className?: string;
}

export function WeatherIcon({ code, className }: WeatherIconProps) {
  const Icon = CODE_TO_ICON[code] ?? BsCloudSlashFill;
  return (
    <span className={className} aria-hidden>
      <Icon />
    </span>
  );
}

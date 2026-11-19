import { type EasingProps, rotationCss, spinClass } from "./easing";
import { SpinnerStyle, spinnerRoot } from "./frame";
import { SIZE } from "./motion";
import type { SpinnerProps } from "./types";

/** Props for the Orbit spinner. */
export interface OrbitProps extends SpinnerProps, EasingProps {}

const css = `
.bds-orbit {
  --bds-orbit-stroke: calc(${SIZE} * 0.0625);
  position: relative;
}

.bds-orbit-track {
  position: absolute;
  inset: 8%;
  border-radius: 9999rem;
  background: conic-gradient(from 180deg, transparent 0deg, currentColor 180deg, transparent 180deg);
  -webkit-mask: radial-gradient(farthest-side, transparent calc(100% - var(--bds-orbit-stroke)), oklch(0 0 0) 0);
  mask: radial-gradient(farthest-side, transparent calc(100% - var(--bds-orbit-stroke)), oklch(0 0 0) 0);
}

.bds-orbit-dot {
  position: absolute;
  top: 50%;
  left: 50%;
  width: 25%;
  height: 25%;
  background: currentColor;
  border-radius: 9999rem;
  transform: translate(-50%, -50%);
}

${rotationCss("orbit")}
`;

/** A fading half-arc rotating around a dot. Decorative: pair it with a visible or screen-reader label. */
export function Orbit({ easing, ...rest }: OrbitProps) {
  return (
    <>
      <SpinnerStyle name="orbit">{css}</SpinnerStyle>
      <div {...spinnerRoot("orbit", rest)}>
        <div className="bds-orbit-dot" />
        <div className={`bds-orbit-track ${spinClass("orbit", easing)}`} />
      </div>
    </>
  );
}

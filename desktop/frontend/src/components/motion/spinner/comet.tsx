import { type EasingProps, rotationCss, spinClass } from "./easing";
import { SpinnerStyle, spinnerRoot } from "./frame";
import { SIZE } from "./motion";
import type { SpinnerProps } from "./types";

/** Props for the Comet spinner. */
export interface CometProps extends SpinnerProps, EasingProps {}

const css = `
.bds-comet {
  --bds-comet-stroke: calc(${SIZE} * 0.12);
  position: relative;
}

.bds-comet-spin {
  position: absolute;
  inset: 0;
}

.bds-comet-tail {
  position: absolute;
  inset: 0;
  border-radius: 9999rem;
  background: conic-gradient(from 0deg, transparent, currentColor);
  -webkit-mask: radial-gradient(farthest-side, transparent calc(100% - var(--bds-comet-stroke)), oklch(0 0 0) 0);
  mask: radial-gradient(farthest-side, transparent calc(100% - var(--bds-comet-stroke)), oklch(0 0 0) 0);
}

.bds-comet-head {
  position: absolute;
  top: 0;
  left: 50%;
  width: var(--bds-comet-stroke);
  height: var(--bds-comet-stroke);
  background: currentColor;
  border-radius: 9999rem;
  transform: translateX(-50%);
}

${rotationCss("comet")}
`;

/** A full ring fading into its tail. Decorative: pair it with a visible or screen-reader label. */
export function Comet({ easing, ...rest }: CometProps) {
  return (
    <>
      <SpinnerStyle name="comet">{css}</SpinnerStyle>
      <div {...spinnerRoot("comet", rest)}>
        <div className={spinClass("comet", easing)}>
          <div className="bds-comet-tail" />
          <div className="bds-comet-head" />
        </div>
      </div>
    </>
  );
}

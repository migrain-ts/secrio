import { type CapProps, linecap } from "./cap";
import { type EasingProps, rotationCss, spinClass } from "./easing";
import { SpinnerStyle, spinnerRoot } from "./frame";
import { animation } from "./motion";
import type { SpinnerProps } from "./types";

/** Props for the Snake spinner. */
export interface SnakeProps extends SpinnerProps, EasingProps, CapProps {}

const css = `
${rotationCss("snake")}

.bds-snake-dash {
  ${animation("snake", "bds-snake-stretch", "ease-in-out")}
}

@keyframes bds-snake-stretch {
  0% {
    stroke-dasharray: 1 100;
    stroke-dashoffset: 0;
  }
  50% {
    stroke-dasharray: 45 100;
    stroke-dashoffset: -17;
  }
  100% {
    stroke-dasharray: 45 100;
    stroke-dashoffset: -62;
  }
}

@media (prefers-reduced-motion: reduce) {
  .bds-snake-dash {
    stroke-dasharray: 18 100;
  }
}
`;

/** An arc stretching and shrinking as it circles. Decorative: pair it with a visible or screen-reader label. */
export function Snake({ cap, easing, ...rest }: SnakeProps) {
  return (
    <>
      <SpinnerStyle name="snake">{css}</SpinnerStyle>
      <svg
        {...spinnerRoot("snake", rest)}
        fill="none"
        role="presentation"
        viewBox="0 0 24 24"
      >
        <g className={spinClass("snake", easing)}>
          <circle
            className="bds-snake-dash"
            cx="12"
            cy="12"
            r="10"
            stroke="currentColor"
            strokeLinecap={linecap(cap)}
            strokeWidth="2.5"
          />
        </g>
      </svg>
    </>
  );
}

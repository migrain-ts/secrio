import { type CapProps, linecap } from "./cap";
import { SpinnerStyle, spinnerRoot, step } from "./frame";
import { animation, stagger } from "./motion";
import type { SpinnerProps } from "./types";

/** Props for the Cascade spinner. */
export interface CascadeProps extends SpinnerProps, CapProps {}

const RADII = [10.5, 7, 3.5];

const SLOTS = 24;

const css = `
.bds-cascade-arc {
  transform-origin: center;
  ${animation("cascade", "bds-cascade-turn", "cubic-bezier(0.68, -0.75, 0.265, 1.75)")}
  animation-delay: ${stagger("cascade", SLOTS)};
}

@keyframes bds-cascade-turn {
  to {
    transform: rotate(360deg);
  }
}
`;

/** Three nested arcs fanning into a spiral and snapping back in line. Decorative: pair it with a visible or screen-reader label. */
export function Cascade({ cap, ...rest }: CascadeProps) {
  return (
    <>
      <SpinnerStyle name="cascade">{css}</SpinnerStyle>
      <svg
        {...spinnerRoot("cascade", rest)}
        fill="none"
        role="presentation"
        stroke="currentColor"
        strokeLinecap={linecap(cap)}
        strokeWidth="2"
        viewBox="0 0 24 24"
      >
        {RADII.map((r, index) => {
          const circumference = 2 * Math.PI * r;
          return (
            <circle
              className="bds-cascade-arc"
              cx="12"
              cy="12"
              key={r}
              r={r}
              strokeDasharray={`${circumference / 4} ${(circumference * 3) / 4}`}
              style={step(index)}
            />
          );
        })}
      </svg>
    </>
  );
}

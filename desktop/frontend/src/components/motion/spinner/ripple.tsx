import { SpinnerStyle, spinnerRoot, step } from "./frame";
import { animation, SIZE, STEP_VAR, stagger } from "./motion";
import type { SpinnerProps } from "./types";

/** Whether the rings spread out from the centre or close in on it. */
export type RippleDirection = "in" | "out";

/** Rings spread outward by default. */
const DEFAULT_RIPPLE_DIRECTION: RippleDirection = "out";

/** Props for the Ripple spinner. */
export interface RippleProps extends SpinnerProps {
  direction?: RippleDirection;
}

const RINGS = Array.from({ length: 3 }, (_, index) => index);

const css = `
.bds-ripple {
  position: relative;
}

.bds-ripple-ring {
  position: absolute;
  inset: 0;
  box-sizing: border-box;
  border: calc(${SIZE} * 0.08) solid currentColor;
  border-radius: 9999rem;
  ${animation("ripple", "bds-ripple-spread", "ease-out")}
  animation-delay: ${stagger("ripple", RINGS.length)};
}

.bds-ripple-ring-in {
  animation-direction: reverse;
}

@keyframes bds-ripple-spread {
  from {
    opacity: 1;
    transform: scale(0);
  }
  to {
    opacity: 0;
    transform: scale(1);
  }
}

@media (prefers-reduced-motion: reduce) {
  .bds-ripple-ring {
    opacity: 0.4;
    transform: scale(calc((var(${STEP_VAR}) + 1) / ${RINGS.length}));
  }
}
`;

/** Three rings spreading out from the center. Decorative: pair it with a visible or screen-reader label. */
export function Ripple({
  direction = DEFAULT_RIPPLE_DIRECTION,
  ...rest
}: RippleProps) {
  return (
    <>
      <SpinnerStyle name="ripple">{css}</SpinnerStyle>
      <div {...spinnerRoot("ripple", rest)}>
        {RINGS.map((index) => (
          <div
            className={`bds-ripple-ring bds-ripple-ring-${direction}`}
            key={index}
            style={step(index)}
          />
        ))}
      </div>
    </>
  );
}

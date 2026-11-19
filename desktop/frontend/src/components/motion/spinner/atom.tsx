import { type EasingProps, rotationCss, spinClass } from "./easing";
import { cssVars, SpinnerStyle, spinnerRoot, step } from "./frame";
import { SIZE, stagger } from "./motion";
import type { SpinnerProps } from "./types";

/** Props for the Atom spinner. */
export interface AtomProps extends SpinnerProps, EasingProps {}

const ORBITS = Array.from({ length: 3 }, (_, index) => index);

const TILT = 180 / ORBITS.length;

const STROKE = `calc(${SIZE} * 0.055)`;

const INNER_STROKE = `calc(${SIZE} * 0.045)`;

const css = `
.bds-atom {
  position: relative;
}

.bds-atom-shell {
  position: absolute;
  inset: 0;
  box-sizing: border-box;
  border: ${STROKE} solid currentColor;
  border-radius: 9999rem;
}

.bds-atom-orbit {
  position: absolute;
  inset: 0;
  transform-style: preserve-3d;
  transform: rotate(var(--bds-atom-tilt)) rotateX(90deg);
}

.bds-atom-ring {
  position: absolute;
  inset: 0;
  box-sizing: border-box;
  border: ${INNER_STROKE} solid currentColor;
  border-radius: 9999rem;
  transform: rotateX(90deg);
}

${rotationCss("atom")}

.bds-atom-spin {
  position: absolute;
  inset: 0;
  transform-style: preserve-3d;
  animation-delay: ${stagger("atom", ORBITS.length)};
}

@media (prefers-reduced-motion: reduce) {
  .bds-atom-spin {
    transform: rotate(${TILT}deg);
  }
}
`;

/** Three rings tumbling inside a circle. Decorative: pair it with a visible or screen-reader label. */
export function Atom({ easing, ...rest }: AtomProps) {
  return (
    <>
      <SpinnerStyle name="atom">{css}</SpinnerStyle>
      <div {...spinnerRoot("atom", rest)}>
        <div className="bds-atom-shell" />
        {ORBITS.map((index) => (
          <div
            className="bds-atom-orbit"
            key={index}
            style={cssVars({ "--bds-atom-tilt": `${index * TILT}deg` })}
          >
            <div className={spinClass("atom", easing)} style={step(index)}>
              <div className="bds-atom-ring" />
            </div>
          </div>
        ))}
      </div>
    </>
  );
}

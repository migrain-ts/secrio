import { cssVars, SpinnerStyle, spinnerRoot, step } from "./frame";
import { animation, stagger } from "./motion";
import type { SpinnerProps } from "./types";

const DOT = 34;

const GAP = 20;

const MARGIN = (100 - 2 * DOT - GAP) / 2;

const FAR = `${(((DOT + GAP) / DOT) * 100).toFixed(1)}%`;

const RESTS = [
  `translate(${FAR}, 0)`,
  "translate(0, 0)",
  `translate(0, ${FAR})`,
];

const css = `
.bds-slide {
  position: relative;
}

.bds-slide-dot {
  position: absolute;
  top: ${MARGIN}%;
  left: ${MARGIN}%;
  width: ${DOT}%;
  height: ${DOT}%;
  background: currentColor;
  border-radius: 50%;
  ${animation("slide", "bds-slide-walk", "ease-in-out")}
  animation-delay: ${stagger("slide", RESTS.length)};
}

@keyframes bds-slide-walk {
  0% {
    transform: translate(${FAR}, 0);
  }
  8.33%,
  25% {
    transform: translate(${FAR}, ${FAR});
  }
  33.33%,
  50% {
    transform: translate(0, ${FAR});
  }
  58.33%,
  75% {
    transform: translate(0, 0);
  }
  83.33%,
  100% {
    transform: translate(${FAR}, 0);
  }
}

@media (prefers-reduced-motion: reduce) {
  .bds-slide-dot {
    transform: var(--bds-slide-rest);
  }
}
`;

/** Three dots sliding into the empty corner of a square. Decorative: pair it with a visible or screen-reader label. */
export function Slide(props: SpinnerProps) {
  return (
    <>
      <SpinnerStyle name="slide">{css}</SpinnerStyle>
      <div {...spinnerRoot("slide", props)}>
        {RESTS.map((rest, index) => (
          <div
            className="bds-slide-dot"
            key={rest}
            style={cssVars({ ...step(index), "--bds-slide-rest": rest })}
          />
        ))}
      </div>
    </>
  );
}

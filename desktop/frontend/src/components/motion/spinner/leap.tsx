import { SpinnerStyle, spinnerRoot, step } from "./frame";
import { animation, SIZE, STEP_VAR, stagger } from "./motion";
import type { SpinnerProps } from "./types";

const DOTS = Array.from({ length: 3 }, (_, index) => index);

const css = `
.bds-leap {
  --bds-leap-dot: round(calc(${SIZE} * 0.22), 0.0625rem);
  --bds-leap-gap: round(down, calc((${SIZE} - var(--bds-leap-dot)) / 2), 0.0625rem);
  position: relative;
}

.bds-leap-wrapper {
  position: absolute;
  top: round(calc((${SIZE} - var(--bds-leap-dot)) / 2), 0.0625rem);
  left: calc(${SIZE} - var(--bds-leap-dot) - var(--bds-leap-gap) * 2);
  width: calc(var(--bds-leap-gap) * 2 + var(--bds-leap-dot));
  height: var(--bds-leap-dot);
  ${animation("leap", "bds-leap-hop", "ease-in-out")}
  animation-delay: ${stagger("leap", DOTS.length)};
}

.bds-leap-dot {
  position: absolute;
  inset: 0 auto 0 0;
  width: var(--bds-leap-dot);
  background: currentColor;
  border-radius: 50%;
}

@keyframes bds-leap-hop {
  0% {
    transform: translateX(0) rotate(0);
  }
  33.33% {
    transform: translateX(0) rotate(180deg);
  }
  66.66% {
    transform: translateX(calc(var(--bds-leap-gap) * -1)) rotate(180deg);
  }
  100% {
    transform: translateX(calc(var(--bds-leap-gap) * -2)) rotate(180deg);
  }
}

@media (prefers-reduced-motion: reduce) {
  .bds-leap-wrapper {
    transform: translateX(calc(var(${STEP_VAR}) * var(--bds-leap-gap)));
  }
}
`;

/** Three dots in a row, the last one leaping to the front. Decorative: pair it with a visible or screen-reader label. */
export function Leap(props: SpinnerProps) {
  const root = spinnerRoot("leap", props);
  return (
    <>
      <SpinnerStyle name="leap">{css}</SpinnerStyle>
      {/* The key restarts the animation when the size changes, because the keyframes round to whole steps of it. */}
      <div aria-hidden={root["aria-hidden"]} className={root.className} key={props.size} style={root.style}>
        {DOTS.map((index) => (
          <div className="bds-leap-wrapper" key={index} style={step(index)}>
            <div className="bds-leap-dot" />
          </div>
        ))}
      </div>
    </>
  );
}

import { SpinnerStyle, spinnerRoot, step } from "./frame";
import { animation, SIZE, stagger } from "./motion";
import type { SpinnerProps } from "./types";

const DOTS = Array.from({ length: 3 }, (_, index) => index);

const css = `
.bds-linear-dots {
  display: flex;
  align-items: center;
  gap: calc(${SIZE} * 0.1875);
  width: auto;
}

.bds-linear-dots-dot {
  width: calc(${SIZE} * 0.1875);
  height: calc(${SIZE} * 0.1875);
  background: currentColor;
  border-radius: 9999rem;
  ${animation("linear-dots", "bds-linear-dots-fade", "linear")}
  animation-delay: ${stagger("linear-dots", DOTS.length)};
}

@keyframes bds-linear-dots-fade {
  0% {
    opacity: 1;
  }
  66.67% {
    opacity: 0.5;
  }
  100% {
    opacity: 1;
  }
}

@media (prefers-reduced-motion: reduce) {
  .bds-linear-dots-dot {
    opacity: 0.75;
  }
}
`;

/** Three dots lighting up in turn from left to right. Decorative: pair it with a visible or screen-reader label. */
export function LinearDots(props: SpinnerProps) {
  return (
    <>
      <SpinnerStyle name="linear-dots">{css}</SpinnerStyle>
      <div {...spinnerRoot("linear-dots", props)}>
        {DOTS.map((dot) => (
          <div className="bds-linear-dots-dot" key={dot} style={step(dot)} />
        ))}
      </div>
    </>
  );
}

import { SpinnerStyle, spinnerRoot, step } from "./frame";
import { animation, SIZE, STEP_VAR, stagger } from "./motion";
import type { SpinnerProps } from "./types";

/** Whether the bars grow from the baseline or from their middle. */
export type WaveOrigin = "bottom" | "center";

/** Bars grow from their middle by default. */
const DEFAULT_WAVE_ORIGIN: WaveOrigin = "center";

/** Props for the Wave spinner. */
export interface WaveProps extends SpinnerProps {
  origin?: WaveOrigin;
}

const BARS = Array.from({ length: 5 }, (_, index) => index);

const css = `
.bds-wave {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.bds-wave-bar {
  width: calc(${SIZE} * 0.12);
  height: 100%;
  background: currentColor;
  border-radius: 9999rem;
  ${animation("wave", "bds-wave-rise", "ease-in-out")}
  animation-delay: ${stagger("wave", BARS.length)};
}

.bds-wave-bar-bottom {
  align-self: flex-end;
}

@keyframes bds-wave-rise {
  0%,
  100% {
    height: 30%;
  }
  50% {
    height: 100%;
  }
}

@media (prefers-reduced-motion: reduce) {
  .bds-wave-bar {
    height: calc((0.4 + var(${STEP_VAR}) * 0.15) * 100%);
  }
}
`;

/** Five bars rising and falling in a wave. Decorative: pair it with a visible or screen-reader label. */
export function Wave({ origin = DEFAULT_WAVE_ORIGIN, ...rest }: WaveProps) {
  return (
    <>
      <SpinnerStyle name="wave">{css}</SpinnerStyle>
      <div {...spinnerRoot("wave", rest)}>
        {BARS.map((index) => (
          <div
            className={`bds-wave-bar bds-wave-bar-${origin}`}
            key={index}
            style={step(index)}
          />
        ))}
      </div>
    </>
  );
}

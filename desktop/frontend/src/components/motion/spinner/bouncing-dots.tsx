import { SpinnerStyle, spinnerRoot, step } from "./frame";
import { animation, SIZE, stagger } from "./motion";
import type { SpinnerProps } from "./types";

const DOTS = Array.from({ length: 3 }, (_, index) => index);

const css = `
.bds-bouncing-dots {
  display: flex;
  align-items: center;
  gap: calc(${SIZE} * 0.2);
  width: auto;
}

.bds-bouncing-dots-dot {
  width: calc(${SIZE} * 0.25);
  height: calc(${SIZE} * 0.25);
  background: currentColor;
  border-radius: 9999rem;
  ${animation("bouncing-dots", "bds-bouncing-dots-bounce", "ease-in-out alternate")}
  animation-delay: ${stagger("bouncing-dots", DOTS.length)};
}

@keyframes bds-bouncing-dots-bounce {
  from {
    transform: translateY(28%);
  }
  to {
    transform: translateY(-72%);
  }
}
`;

/** Three staggered dots bouncing up and down. Decorative: pair it with a visible or screen-reader label. */
export function BouncingDots(props: SpinnerProps) {
  return (
    <>
      <SpinnerStyle name="bouncing-dots">{css}</SpinnerStyle>
      <div {...spinnerRoot("bouncing-dots", props)}>
        {DOTS.map((dot) => (
          <div className="bds-bouncing-dots-dot" key={dot} style={step(dot)} />
        ))}
      </div>
    </>
  );
}

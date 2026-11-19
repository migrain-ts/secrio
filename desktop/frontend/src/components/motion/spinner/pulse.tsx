import { SpinnerStyle, spinnerRoot } from "./frame";
import { animation } from "./motion";
import type { SpinnerProps } from "./types";

const css = `
.bds-pulse-ring {
  transform-origin: center;
  ${animation("pulse", "bds-pulse-ripple", "ease-out")}
}

@keyframes bds-pulse-ripple {
  from {
    opacity: 0.4;
    transform: scale(0.25);
  }
  to {
    opacity: 0;
    transform: scale(1);
  }
}

@media (prefers-reduced-motion: reduce) {
  .bds-pulse-ring {
    opacity: 0.2;
  }
}
`;

/** A ring rippling outward from a dot. Decorative: pair it with a visible or screen-reader label. */
export function Pulse(props: SpinnerProps) {
  return (
    <>
      <SpinnerStyle name="pulse">{css}</SpinnerStyle>
      <svg
        {...spinnerRoot("pulse", props)}
        fill="none"
        role="presentation"
        viewBox="0 0 16 16"
      >
        <circle
          className="bds-pulse-ring"
          cx="8"
          cy="8"
          fill="currentColor"
          r="8"
        />
        <circle cx="8" cy="8" fill="currentColor" r="2" />
      </svg>
    </>
  );
}

import { SpinnerStyle, spinnerRoot } from "./frame";
import { animation, SIZE } from "./motion";
import type { SpinnerProps } from "./types";

const css = `
.bds-flip {
  perspective: calc(${SIZE} * 3);
}

.bds-flip-face {
  width: 64%;
  height: 64%;
  margin: 18%;
  background: currentColor;
  border-radius: calc(${SIZE} * 0.06);
  ${animation("flip", "bds-flip-turn", "ease-in-out")}
}

@keyframes bds-flip-turn {
  0% {
    transform: rotateX(0) rotateY(0);
  }
  50% {
    transform: rotateX(-180deg) rotateY(0);
  }
  100% {
    transform: rotateX(-180deg) rotateY(-180deg);
  }
}
`;

/** A square flipping over on one axis, then the other. Decorative: pair it with a visible or screen-reader label. */
export function Flip(props: SpinnerProps) {
  return (
    <>
      <SpinnerStyle name="flip">{css}</SpinnerStyle>
      <div {...spinnerRoot("flip", props)}>
        <div className="bds-flip-face" />
      </div>
    </>
  );
}

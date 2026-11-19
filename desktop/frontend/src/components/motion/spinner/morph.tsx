import { SpinnerStyle, spinnerRoot } from "./frame";
import { animation, SIZE } from "./motion";
import type { SpinnerProps } from "./types";

const css = `
.bds-morph-shape {
  width: 64%;
  height: 64%;
  margin: 18%;
  background: currentColor;
  border-radius: calc(${SIZE} * 0.08);
  ${animation("morph", "bds-morph-round", "ease-in-out")}
}

@keyframes bds-morph-round {
  0% {
    border-radius: calc(${SIZE} * 0.08);
    transform: rotate(0);
  }
  50% {
    border-radius: 50%;
    transform: rotate(45deg);
  }
  100% {
    border-radius: calc(${SIZE} * 0.08);
    transform: rotate(90deg);
  }
}
`;

/** A square rounding into a circle and back as it turns. Decorative: pair it with a visible or screen-reader label. */
export function Morph(props: SpinnerProps) {
  return (
    <>
      <SpinnerStyle name="morph">{css}</SpinnerStyle>
      <div {...spinnerRoot("morph", props)}>
        <div className="bds-morph-shape" />
      </div>
    </>
  );
}

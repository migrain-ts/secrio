import { animation, type SpinnerName } from "./motion";

/** How a rotation is paced. `stacked` adds an eased turn on top of the linear one, so the loop surges once per revolution. */
export type Easing = "linear" | "ease-in-out" | "stacked";

/** Rotation is linear by default: a loop has no start or end, so easing would pulse at the seam. */
const DEFAULT_EASING: Easing = "linear";

/** The pacing option a rotating spinner accepts. */
export interface EasingProps {
  easing?: Easing;
}

/** Returns the class names that select a rotation pacing. */
export function spinClass(
  name: SpinnerName,
  easing: Easing = DEFAULT_EASING
): string {
  const base = `bds-${name}-spin`;
  return easing === "linear" ? base : `${base} ${base}-${easing}`;
}

const ROTATE = `to {
    transform: rotate(360deg);
  }`;

/** Builds the rotation rules and keyframes for one spinner, with all three pacings. */
export function rotationCss(name: SpinnerName, turn = ROTATE): string {
  return `
.bds-${name}-spin {
  transform-origin: center;
  ${animation(name, `bds-${name}-rotate`, "linear")}
}

.bds-${name}-spin-ease-in-out {
  animation-timing-function: ease-in-out;
}

.bds-${name}-spin-stacked {
  animation-name: bds-${name}-rotate, bds-${name}-rotate;
  animation-timing-function: linear, ease-in-out;
  animation-composition: add;
}

@keyframes bds-${name}-rotate {
  ${turn}
}
`;
}

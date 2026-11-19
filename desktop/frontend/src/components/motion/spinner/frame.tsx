import type { CSSProperties } from "react";

import {
  DEFAULT_SIZE,
  DURATION_VAR,
  PLAY_STATE_VAR,
  SIZE,
  SIZE_VAR,
  type SpinnerName,
  STEP_VAR,
} from "./motion";
import type { SpinnerProps } from "./types";

// React 19 hoists a style element that carries `href` and `precedence`, and
// writes it once however many spinners render. React 18 has no such props in
// its types and renders the element in place, which still works. Spreading
// the props keeps one source compiling under both.
interface HoistedStyleProps {
  href: string;
  precedence: string;
}

function hoisted(name: SpinnerName): HoistedStyleProps {
  return { href: `bds-${name}`, precedence: "better-design-spinner" };
}

/** Writes the stylesheet of one shape: its size, its keyframes and the rule that stops it under reduced motion. */
export function SpinnerStyle({ children, name }: { children: string; name: SpinnerName }) {
  return (
    <style {...hoisted(name)}>
      {`
.bds-${name} {
  width: ${SIZE};
  height: ${SIZE};
}
${children}
@media (prefers-reduced-motion: reduce) {
  .bds-${name},
  .bds-${name} * {
    animation: none;
  }
}
`}
    </style>
  );
}

/** Types an inline style object that sets spinner custom properties. */
export function cssVars(vars: CSSProperties & Record<`--bds-${string}`, number | string>): CSSProperties {
  return vars;
}

/** Inline style that gives one element its position in a staggered sequence. */
export function step(index: number): CSSProperties {
  return cssVars({ [STEP_VAR]: index });
}

/** Root attributes every shape shares: hidden from assistive technology, sized, and coloured only when asked. */
export function spinnerRoot(
  name: SpinnerName,
  { className, color, duration, playState, size = DEFAULT_SIZE }: SpinnerProps,
) {
  return {
    "aria-hidden": true,
    className: [`bds-${name}`, className].filter(Boolean).join(" "),
    style: cssVars({
      [SIZE_VAR]: `${size}rem`,
      ...(color === undefined ? null : { color }),
      ...(duration === undefined ? null : { [DURATION_VAR]: `${duration}ms` }),
      ...(playState === undefined ? null : { [PLAY_STATE_VAR]: playState }),
    }),
  };
}

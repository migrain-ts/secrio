import type { ComponentType } from "react";

import { cn } from "@/lib/utils";

import { Arc } from "./spinner/arc";
import { Atom } from "./spinner/atom";
import { Blocks } from "./spinner/blocks";
import { BouncingDots } from "./spinner/bouncing-dots";
import { Cascade } from "./spinner/cascade";
import { CircularDots } from "./spinner/circular-dots";
import { Classic } from "./spinner/classic";
import { ClassicV2 } from "./spinner/classic-v2";
import { Clock } from "./spinner/clock";
import { Comet } from "./spinner/comet";
import { Compass } from "./spinner/compass";
import { Dual } from "./spinner/dual";
import { Eclipse } from "./spinner/eclipse";
import { Flip } from "./spinner/flip";
import { Gather } from "./spinner/gather";
import { Leap } from "./spinner/leap";
import { LinearDots } from "./spinner/linear-dots";
import { Morph } from "./spinner/morph";
import { Orbit } from "./spinner/orbit";
import { PixelRing } from "./spinner/pixel-ring";
import { Pulse } from "./spinner/pulse";
import { Radar } from "./spinner/radar";
import { Ring } from "./spinner/ring";
import { Ripple } from "./spinner/ripple";
import { Slide } from "./spinner/slide";
import { Snake } from "./spinner/snake";
import { Swirl } from "./spinner/swirl";
import { Trace } from "./spinner/trace";
import { Wave } from "./spinner/wave";
import type { SpinnerName } from "./spinner/motion";
import type { SpinnerProps as VariantProps } from "./spinner/types";

/** Every spinner shape, keyed by the name the `variant` prop takes. */
export const SPINNER_VARIANTS = {
  arc: Arc,
  atom: Atom,
  blocks: Blocks,
  "bouncing-dots": BouncingDots,
  cascade: Cascade,
  "circular-dots": CircularDots,
  classic: Classic,
  "classic-v2": ClassicV2,
  clock: Clock,
  comet: Comet,
  compass: Compass,
  dual: Dual,
  eclipse: Eclipse,
  flip: Flip,
  gather: Gather,
  leap: Leap,
  "linear-dots": LinearDots,
  morph: Morph,
  orbit: Orbit,
  "pixel-ring": PixelRing,
  pulse: Pulse,
  radar: Radar,
  ring: Ring,
  ripple: Ripple,
  slide: Slide,
  snake: Snake,
  swirl: Swirl,
  trace: Trace,
  wave: Wave,
} satisfies Record<SpinnerName, ComponentType<VariantProps>>;

/** The name of one spinner shape. */
export type SpinnerVariant = keyof typeof SPINNER_VARIANTS;

/** Props for the spinner. Options such as `easing` or `cap` live on the shape: import it from `./spinner/<name>`. */
export type SpinnerProps = VariantProps & {
  /** The shape to draw. Defaults to `arc`. */
  variant?: SpinnerVariant;
  /**
   * What a screen reader announces. Pass `null` when the spinner sits beside
   * visible text that already says the work is running, such as a busy button.
   */
  label?: string | null;
};

/** Shows that work is in progress, in one of 29 shapes. It takes the text colour around it. */
export function Spinner({ variant = "arc", label = "Loading", className, ...rest }: SpinnerProps) {
  const Variant: ComponentType<VariantProps> = SPINNER_VARIANTS[variant];
  if (label === null) return <Variant className={className} {...rest} />;
  // `className` goes on the outermost element in both cases, so a layout class behaves the same.
  return (
    <div role="status" className={cn("inline-flex", className)}>
      <Variant {...rest} />
      <span className="sr-only">{label}</span>
    </div>
  );
}

export const DURATION_VAR = "--bds-duration";

/** Custom property that pauses or runs the animation. An ancestor can set it for every spinner inside. */
export const PLAY_STATE_VAR = "--bds-play-state";

/** Custom property that holds the spinner width and height. */
export const SIZE_VAR = "--bds-size";

/** Custom property that holds the position of one element in a staggered sequence. */
export const STEP_VAR = "--bds-step";

/** Default width and height in rem. */
export const DEFAULT_SIZE = 1.25;

/** CSS value for the spinner size, with the default as its fallback. */
export const SIZE = `var(${SIZE_VAR}, ${DEFAULT_SIZE}rem)`;

/** Length of one loop per shape, in milliseconds. Each value is tuned to the shape: a hop reads faster than a sweep. */
export const SPINNER_MOTION = {
  arc: 800,
  atom: 1000,
  blocks: 1300,
  "bouncing-dots": 500,
  cascade: 1500,
  "circular-dots": 800,
  classic: 1200,
  "classic-v2": 800,
  clock: 1200,
  comet: 700,
  compass: 500,
  dual: 1000,
  eclipse: 1200,
  flip: 1200,
  gather: 1600,
  leap: 1800,
  "linear-dots": 900,
  morph: 1200,
  orbit: 750,
  "pixel-ring": 1000,
  pulse: 1200,
  radar: 1500,
  ring: 800,
  ripple: 1200,
  slide: 2400,
  snake: 1400,
  swirl: 1200,
  trace: 1200,
  wave: 900,
} as const satisfies Record<string, number>;

/** The name of one spinner shape. */
export type SpinnerName = keyof typeof SPINNER_MOTION;

/** CSS value for the loop length of a shape, overridable through the duration property. */
export function duration(name: SpinnerName): string {
  return `var(${DURATION_VAR}, ${SPINNER_MOTION[name]}ms)`;
}

/** CSS delay that spreads `count` elements evenly across one loop, read from each element's step. */
export function stagger(name: SpinnerName, count: number): string {
  return `calc(${duration(name)} * (var(${STEP_VAR}) - ${count}) / ${count})`;
}

/** CSS value for the play state, running unless a caller pauses it. */
const PLAY_STATE = `var(${PLAY_STATE_VAR}, running)`;

/** Builds the `animation` declaration for one or more keyframe sets, wired to the duration and play-state properties. */
export function animation(
  name: SpinnerName,
  keyframes: string | string[],
  timing: string
): string {
  const runs = [keyframes]
    .flat()
    .map((frames) => `${frames} ${duration(name)} ${timing} infinite`);
  return `animation: ${runs.join(", ")};
  animation-play-state: ${PLAY_STATE};`;
}

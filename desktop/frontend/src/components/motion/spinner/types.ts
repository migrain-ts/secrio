export interface SpinnerProps {
  className?: string;
  /** Any CSS colour. Leave it out to inherit the text colour. */
  color?: string;
  /** Length of one loop in milliseconds. Leave it out to keep the pace tuned for the shape. */
  duration?: number;
  /** Pause the loop without unmounting it, for example while the tab is hidden. */
  playState?: "paused" | "running";
  /** Width and height in rem. Defaults to 1.25. */
  size?: number;
}

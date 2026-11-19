export type Cap = "flat" | "round";

/** Strokes end round unless a caller asks for flat. */
const DEFAULT_CAP: Cap = "round";

/** The stroke-end option a stroked spinner accepts. */
export interface CapProps {
  cap?: Cap;
}

/** Maps the cap option to the SVG `stroke-linecap` value. */
export function linecap(cap: Cap = DEFAULT_CAP): "butt" | "round" {
  return cap === "flat" ? "butt" : "round";
}

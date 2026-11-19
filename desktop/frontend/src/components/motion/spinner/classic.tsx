import { fadeCss } from "./fade";
import { SpinnerStyle, spinnerRoot, step } from "./frame";
import { SIZE, STEP_VAR } from "./motion";
import type { SpinnerProps } from "./types";

const BARS = Array.from({ length: 12 }, (_, index) => index);

const css = `
.bds-classic-inner {
  position: relative;
  top: 50%;
  left: 50%;
  width: ${SIZE};
  height: ${SIZE};
}

.bds-classic-bar {
  position: absolute;
  top: -3.9%;
  left: -10%;
  width: 24%;
  height: 8%;
  background: currentColor;
  border-radius: 0.375rem;
  transform: rotate(calc(var(${STEP_VAR}) * 30deg)) translate(146%);
}

${fadeCss("classic", "bar", BARS.length, { dim: 0.15, rest: 0.5 })}
`;

/** Twelve fading bars arranged in a radial pattern. Decorative: pair it with a visible or screen-reader label. */
export function Classic(props: SpinnerProps) {
  return (
    <>
      <SpinnerStyle name="classic">{css}</SpinnerStyle>
      <div {...spinnerRoot("classic", props)}>
        <div className="bds-classic-inner">
          {BARS.map((bar) => (
            <div className="bds-classic-bar" key={bar} style={step(bar)} />
          ))}
        </div>
      </div>
    </>
  );
}

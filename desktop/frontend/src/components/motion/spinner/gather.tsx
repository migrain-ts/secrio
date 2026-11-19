import { cssVars, SpinnerStyle, spinnerRoot } from "./frame";
import { animation, SIZE } from "./motion";
import type { SpinnerProps } from "./types";

const BLOCKS = [
  { x: 1, y: 1 },
  { x: -1, y: 1 },
  { x: 1, y: -1 },
  { x: -1, y: -1 },
];

const GAP = 24;

const GAP_IN = 8;

const BLOCK = (100 - GAP) / 2;

const PULL = `${(((GAP - GAP_IN) / 2 / BLOCK) * 100).toFixed(1)}%`;

const css = `
.bds-gather-group {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: ${GAP}%;
  width: 100%;
  height: 100%;
  ${animation("gather", "bds-gather-turn", "ease-in-out")}
}

.bds-gather-block {
  background: currentColor;
  border-radius: calc(${SIZE} * 0.14);
  ${animation("gather", "bds-gather-pull", "ease-in-out")}
}

@keyframes bds-gather-pull {
  0%,
  100% {
    transform: translate(0, 0);
  }
  30%,
  60% {
    transform: translate(
      calc(var(--bds-gather-x) * ${PULL}),
      calc(var(--bds-gather-y) * ${PULL})
    );
  }
}

@keyframes bds-gather-turn {
  0%,
  30% {
    transform: rotate(0);
  }
  60%,
  100% {
    transform: rotate(90deg);
  }
}
`;

/** Four blocks pulling together, turning, and pushing apart. Decorative: pair it with a visible or screen-reader label. */
export function Gather(props: SpinnerProps) {
  return (
    <>
      <SpinnerStyle name="gather">{css}</SpinnerStyle>
      <div {...spinnerRoot("gather", props)}>
        <div className="bds-gather-group">
          {BLOCKS.map(({ x, y }) => (
            <div
              className="bds-gather-block"
              key={`${x}${y}`}
              style={cssVars({ "--bds-gather-x": x, "--bds-gather-y": y })}
            />
          ))}
        </div>
      </div>
    </>
  );
}

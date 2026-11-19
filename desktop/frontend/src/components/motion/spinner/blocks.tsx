import { SpinnerStyle, spinnerRoot, step } from "./frame";
import { animation, SIZE, stagger } from "./motion";
import type { SpinnerProps } from "./types";

/** The direction the shrink sweeps across the grid. */
export type BlocksSweep = "columns" | "diagonal" | "rows";

/** The sweep runs corner to corner by default. */
const DEFAULT_BLOCKS_SWEEP: BlocksSweep = "diagonal";

/** Props for the Blocks spinner. */
export interface BlocksProps extends SpinnerProps {
  sweep?: BlocksSweep;
}

const SIDE = 3;

interface Cell {
  col: number;
  row: number;
}

const CELLS: Cell[] = Array.from({ length: SIDE * SIDE }, (_, index) => ({
  col: index % SIDE,
  row: Math.floor(index / SIDE),
}));

const SWEEPS = {
  columns: { count: SIDE, place: ({ col }) => col },
  diagonal: { count: SIDE * 2 - 1, place: ({ col, row }) => row + col },
  rows: { count: SIDE, place: ({ row }) => row },
} satisfies Record<BlocksSweep, { count: number; place: (cell: Cell) => number }>;

const css = `
.bds-blocks {
  display: grid;
  grid-template-columns: repeat(${SIDE}, 1fr);
  grid-template-rows: repeat(${SIDE}, 1fr);
  gap: calc(${SIZE} * 0.1);
}

.bds-blocks-cell {
  background: currentColor;
  border-radius: calc(${SIZE} * 0.0625);
  ${animation("blocks", "bds-blocks-sweep", "ease-in-out")}
}
${Object.entries(SWEEPS)
  .map(
    ([sweep, { count }]) => `
.bds-blocks-cell-${sweep} {
  animation-delay: ${stagger("blocks", count)};
}
`
  )
  .join("")}
@keyframes bds-blocks-sweep {
  0%,
  70%,
  100% {
    transform: scale(1);
  }
  35% {
    transform: scale(0);
  }
}

@media (prefers-reduced-motion: reduce) {
  .bds-blocks-cell {
    transform: scale(0.8);
  }
}
`;

/** Nine blocks shrinking and growing in a sweep across a grid. Decorative: pair it with a visible or screen-reader label. */
export function Blocks({ sweep = DEFAULT_BLOCKS_SWEEP, ...rest }: BlocksProps) {
  const { place } = SWEEPS[sweep];
  return (
    <>
      <SpinnerStyle name="blocks">{css}</SpinnerStyle>
      <div {...spinnerRoot("blocks", rest)}>
        {CELLS.map((cell) => (
          <div
            className={`bds-blocks-cell bds-blocks-cell-${sweep}`}
            key={`${cell.col}-${cell.row}`}
            style={step(place(cell))}
          />
        ))}
      </div>
    </>
  );
}

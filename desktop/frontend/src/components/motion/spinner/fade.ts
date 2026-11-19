import { animation, type SpinnerName, stagger } from "./motion";

/** Builds the staggered fade every stepped spinner shares, with its reduced-motion resting opacity. */
export function fadeCss(
  name: SpinnerName,
  element: string,
  count: number,
  { dim, rest }: { dim: number; rest: number }
): string {
  return `
.bds-${name}-${element} {
  ${animation(name, `bds-${name}-fade`, "linear")}
  animation-delay: ${stagger(name, count)};
}

@keyframes bds-${name}-fade {
  from {
    opacity: 1;
  }
  to {
    opacity: ${dim};
  }
}

@media (prefers-reduced-motion: reduce) {
  .bds-${name}-${element} {
    opacity: ${rest};
  }
}
`;
}

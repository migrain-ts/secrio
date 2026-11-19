// Used by vite.config.ts: a production build must never be configured for the
// mock api. VITE_MOCK_API is baked in at build time, so a stray value during
// `wails build` would otherwise ship a mock-only app.

/** Throws when VITE_MOCK_API is set (non-empty) for any mode other than
 *  development or test. */
export function assertNoMockInProduction(mode: string, env: Record<string, string | undefined>): void {
  if (mode === 'development' || mode === 'test') return
  if (env.VITE_MOCK_API) {
    throw new Error(
      `VITE_MOCK_API is set (${JSON.stringify(env.VITE_MOCK_API)}) for a "${mode}" build. ` +
        'The mock api is for the dev server and tests only; unset VITE_MOCK_API and rebuild.',
    )
  }
}

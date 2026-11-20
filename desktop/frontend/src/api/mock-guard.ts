// Used by vite.config.ts: a production build must never be configured for the
// mock api. VITE_MOCK_API is baked in at build time, so a stray value during
// `wails build` would otherwise ship a mock-only app.

/**
 * Throws for a `vite build` that is not a production build: mode test or
 * development (import.meta.env.DEV/MODE would allow the mock, see select.ts),
 * or NODE_ENV set to anything other than production. Vitest loads this config
 * with command 'serve', so tests are unaffected.
 */
export function assertProductionBuild(command: string, mode: string, nodeEnv: string | undefined): void {
  if (command !== 'build') return
  if (mode === 'test' || mode === 'development') {
    throw new Error(`Refusing a "${mode}" vite build: release builds use the default production mode.`)
  }
  if (nodeEnv && nodeEnv !== 'production') {
    throw new Error(
      `NODE_ENV is ${JSON.stringify(nodeEnv)} for a vite build. Unset NODE_ENV (or set it to production) and rebuild.`,
    )
  }
}

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

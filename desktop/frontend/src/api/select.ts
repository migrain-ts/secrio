// Pure choice of the backend behind `api` (see client.ts). Kept free of any
// import of the mock so production can tree-shake the mock away.

/** real: the Wails bindings; mock: in-memory (dev/test only); unavailable:
 *  production without bindings. The app must show a fatal "backend
 *  unavailable" screen for 'unavailable' and never fall back to the mock. */
export type BackendStatus = 'real' | 'mock' | 'unavailable'

export interface SelectInput {
  /** import.meta.env.DEV */
  dev: boolean
  /** import.meta.env.MODE */
  mode: string
  /** VITE_MOCK_API === '1' */
  forceMock: boolean
  /** window.go.desktop.API exists */
  hasBindings: boolean
}

/** Production (any mode other than dev/test) never selects the mock. */
export function selectApi({ dev, mode, forceMock, hasBindings }: SelectInput): BackendStatus {
  const mockAllowed = dev || mode === 'test'
  if (!mockAllowed) return hasBindings ? 'real' : 'unavailable'
  if (forceMock) return 'mock'
  return hasBindings ? 'real' : 'mock'
}

export function hasBindings(win: unknown): boolean {
  const go = (win as { go?: { desktop?: { API?: unknown } } } | undefined)?.go
  return Boolean(go?.desktop?.API)
}

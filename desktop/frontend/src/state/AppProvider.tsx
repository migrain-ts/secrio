// App state for the whole UI: the current screen, the current vault and a
// guarded api. Every screen talks to Go through useApp().api, never through
// the raw `api`, so a `locked` error from any call returns to the unlock screen.
import {
  createContext,
  type ReactNode,
  type RefObject,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useReducer,
  useRef,
  useState,
} from 'react'
import { api as realApi, API_METHODS, type Api, backendStatus as realStatus } from '@/api/client'
import { ApiError } from '@/api/errors'
import type { BackendStatus } from '@/api/select'
import type { CurrentVault, VaultDTO } from '@/api/types'
import { type AppEvent, initialState, isDataScreen, type Notice, reducer, type Screen } from '@/state/machine'

export interface VaultSync {
  current: CurrentVault
  vaults: VaultDTO[]
}

interface AppContextValue {
  screen: Screen
  dispatch: (ev: AppEvent) => void
  /** The current vault, null when none is selected. */
  vault: VaultDTO | null
  /** Every vault, sorted by name (from the last sync). */
  vaults: VaultDTO[]
  /** The guarded api: screens use this, never the raw one. */
  api: Api
  /** Re-reads the current vault and the vault list. Call it at boot and after
   *  a user action (unlock, create, switch), never in a timer. */
  syncVault(): Promise<VaultSync>
}

const AppContext = createContext<AppContextValue | null>(null)

export function useApp(): AppContextValue {
  const v = useContext(AppContext)
  if (!v) throw new Error('useApp must be used inside <AppProvider>')
  return v
}

/** Leading-edge throttle of the focus check. */
export const FOCUS_CHECK_MIN_MS = 2000

type AnyFn = (...args: unknown[]) => Promise<unknown>
type Dispatch = (ev: AppEvent) => void

interface Props {
  /** Injected in tests (createMockApi); defaults to the real api. */
  api?: Api
  backendStatus?: BackendStatus
  children: ReactNode
}

export function AppProvider({ api: raw = realApi, backendStatus = realStatus, children }: Props) {
  const [state, dispatch] = useReducer(reducer, initialState)
  const [vault, setVault] = useState<VaultDTO | null>(null)
  const [vaults, setVaults] = useState<VaultDTO[]>([])

  // Refs so the guarded api and the focus listener read the latest values
  // without being rebuilt on every render.
  const screenRef = useRef(state.screen)
  const vaultRef = useRef(vault)
  useEffect(() => {
    screenRef.current = state.screen
    vaultRef.current = vault
  })

  const lockedOut = useCallback((notice: Notice) => {
    dispatch({ type: 'LOCKED', vaultId: vaultRef.current?.id ?? null, returnTo: screenRef.current, notice })
  }, [])

  const api = useMemo<Api>(() => {
    const onError = (e: unknown) => {
      if (!(e instanceof ApiError)) return
      // an action failed: whatever it was doing was not saved
      if (e.code === 'locked') lockedOut('interrupted')
      else if (e.code === 'no_vault' && isDataScreen(screenRef.current)) {
        raw.listVaults().then(
          (vs) => dispatch(vs.length > 0 ? { type: 'GOTO_UNLOCK', vaultId: null } : { type: 'GOTO_CREATE_VAULT' }),
          () => dispatch({ type: 'GOTO_UNLOCK', vaultId: null }),
        )
      }
    }
    const guard =
      (f: AnyFn): AnyFn =>
      async (...args) => {
        try {
          return await f(...args)
        } catch (e) {
          onError(e)
          throw e // never swallowed: the caller still sees it
        }
      }
    return Object.fromEntries(API_METHODS.map((m) => [m, guard((raw[m] as AnyFn).bind(raw))])) as unknown as Api
  }, [raw, lockedOut])

  const syncVault = useCallback(async (): Promise<VaultSync> => {
    const [current, vs] = await Promise.all([api.currentVault(), api.listVaults()])
    setVault(current.vault.id === '' ? null : current.vault)
    setVaults(vs)
    vaultRef.current = current.vault.id === '' ? null : current.vault
    return { current, vaults: vs }
  }, [api])

  useBoot(backendStatus, syncVault, dispatch)
  useFocusCheck(screenRef, api, lockedOut)

  const value = useMemo(
    () => ({ screen: state.screen, dispatch, vault, vaults, api, syncVault }),
    [state.screen, vault, vaults, api, syncVault],
  )
  return <AppContext.Provider value={value}>{children}</AppContext.Provider>
}

/** Picks the first screen. 'unavailable' goes to fatal WITHOUT any api call. */
function useBoot(backendStatus: BackendStatus, syncVault: () => Promise<VaultSync>, dispatch: Dispatch) {
  const started = useRef(false)
  useEffect(() => {
    if (started.current) return // StrictMode runs effects twice in dev
    started.current = true
    if (backendStatus === 'unavailable') {
      dispatch({ type: 'FATAL' })
      return
    }
    syncVault().then(
      ({ current, vaults }) => {
        if (current.unlocked) dispatch({ type: 'BOOTED', screen: { kind: 'projects' } })
        else if (current.vault.id !== '')
          dispatch({ type: 'BOOTED', screen: { kind: 'unlock', vaultId: current.vault.id } })
        else if (vaults.length === 0) dispatch({ type: 'BOOTED', screen: { kind: 'create-vault' } })
        else dispatch({ type: 'BOOTED', screen: { kind: 'unlock', vaultId: null } })
      },
      () => dispatch({ type: 'FATAL' }),
    )
  }, [backendStatus, syncVault, dispatch])
}

/**
 * On window focus, while a data screen is shown, checks ONCE whether the
 * session is still open (leading-edge throttle of FOCUS_CHECK_MIN_MS). There is
 * deliberately no timer anywhere: currentVault refreshes the session idle
 * timer, so a poll would keep the vault unlocked forever (Api.currentVault).
 */
function useFocusCheck(screenRef: RefObject<Screen>, api: Api, lockedOut: (notice: Notice) => void) {
  const last = useRef(-Infinity)
  useEffect(() => {
    const onFocus = () => {
      if (!isDataScreen(screenRef.current)) return
      const now = Date.now()
      if (now - last.current < FOCUS_CHECK_MIN_MS) return
      last.current = now
      api.currentVault().then(
        (cv) => {
          if (!cv.unlocked) lockedOut('locked')
        },
        () => {}, // the guarded api already reacted to locked/no_vault
      )
    }
    window.addEventListener('focus', onFocus)
    return () => window.removeEventListener('focus', onFocus)
  }, [screenRef, api, lockedOut])
}

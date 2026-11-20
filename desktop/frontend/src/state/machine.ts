// The app's screens and how events move between them. A pure reducer: no api
// call, no timer. Boot decisions live in AppProvider (useBoot).

/** Why the unlock screen is shown: 'interrupted' = an action failed because
 *  the vault was locked (its change was not saved); 'locked' = the focus check
 *  found the session closed. The Lock button shows no notice. */
export type Notice = 'interrupted' | 'locked'

export type DataScreen = { kind: 'projects' } | { kind: 'project'; projectId: string; envId: string | null }

export type Screen =
  | { kind: 'boot' }
  | { kind: 'fatal' }
  | { kind: 'unlock'; vaultId: string | null; notice?: Notice }
  | { kind: 'create-vault' }
  | DataScreen

export type BootTarget = Extract<Screen, { kind: 'projects' | 'unlock' | 'create-vault' }>

export type AppEvent =
  | { type: 'BOOTED'; screen: BootTarget }
  /** The session closed (Lock button, a `locked` error, a focus check).
   *  returnTo is the screen to restore after unlocking the same vault. */
  | { type: 'LOCKED'; vaultId: string | null; returnTo: Screen; notice?: Notice }
  | { type: 'UNLOCKED'; vaultId: string }
  | { type: 'VAULT_CREATED' }
  | { type: 'OPEN_PROJECT'; projectId: string; envId?: string | null }
  /** On the project screen: the env shown (kept to restore after unlock). */
  | { type: 'SELECT_ENV'; envId: string | null }
  | { type: 'OPEN_PROJECTS' }
  | { type: 'GOTO_CREATE_VAULT' }
  | { type: 'GOTO_UNLOCK'; vaultId: string | null }
  | { type: 'FATAL' }

export interface AppState {
  screen: Screen
  /** Where to go back after unlocking vaultId again. */
  returnTo: { vaultId: string; screen: DataScreen } | null
}

export const initialState: AppState = { screen: { kind: 'boot' }, returnTo: null }

export function isDataScreen(s: Screen): s is DataScreen {
  return s.kind === 'projects' || s.kind === 'project'
}

const PROJECTS: DataScreen = { kind: 'projects' }

export function reducer(state: AppState, ev: AppEvent): AppState {
  if (state.screen.kind === 'fatal') return state // terminal: restart the app
  switch (ev.type) {
    case 'BOOTED':
      return state.screen.kind === 'boot' ? { ...state, screen: ev.screen } : state
    case 'LOCKED': {
      // Only a data screen can be locked out of; unlock/create/boot stay put.
      if (!isDataScreen(state.screen)) return state
      const returnTo = isDataScreen(ev.returnTo) && ev.vaultId ? { vaultId: ev.vaultId, screen: ev.returnTo } : null
      const screen: Screen = ev.notice
        ? { kind: 'unlock', vaultId: ev.vaultId, notice: ev.notice }
        : { kind: 'unlock', vaultId: ev.vaultId }
      return { screen, returnTo }
    }
    case 'UNLOCKED': {
      const back = state.returnTo?.vaultId === ev.vaultId ? state.returnTo.screen : PROJECTS
      return { screen: back, returnTo: null }
    }
    case 'VAULT_CREATED':
      return { screen: PROJECTS, returnTo: null }
    case 'OPEN_PROJECT':
      return { ...state, screen: { kind: 'project', projectId: ev.projectId, envId: ev.envId ?? null } }
    case 'OPEN_PROJECTS':
      return { ...state, screen: PROJECTS }
    case 'SELECT_ENV':
      return state.screen.kind === 'project' ? { ...state, screen: { ...state.screen, envId: ev.envId } } : state
    case 'GOTO_CREATE_VAULT':
      return { ...state, screen: { kind: 'create-vault' } }
    case 'GOTO_UNLOCK':
      return { ...state, screen: { kind: 'unlock', vaultId: ev.vaultId } }
    case 'FATAL':
      return { screen: { kind: 'fatal' }, returnTo: null }
  }
}

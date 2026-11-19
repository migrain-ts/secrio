// The one entry point the UI uses to talk to Go. Never import wailsjs/runtime
// here: it throws outside Wails (see docs/superpowers/notes/2026-11-19-wails-spike.md).
import * as B from '../../wailsjs/go/desktop/API'
import { parseApiError } from '@/api/errors'
import { createMockApi } from '@/api/mock'
import type {
  CopyResult,
  CurrentVault,
  EnvDTO,
  ImportPreview,
  ImportStrategy,
  ProjectDTO,
  SecretDTO,
  SecretType,
  VaultDTO,
} from '@/api/types'

/** The bound methods of internal/desktop/api.go. Every method rejects with an
 *  ApiError (see errors.ts), never with anything else. */
export interface Api {
  listVaults(): Promise<VaultDTO[]>
  /** Creates, unlocks and selects the vault. */
  createVault(name: string, password: string): Promise<VaultDTO>
  unlock(vaultId: string, password: string): Promise<void>
  lock(): Promise<void>
  selectVault(vaultId: string): Promise<void>
  /**
   * The current vault and whether its session is open. This call refreshes the
   * Core session idle timer, so the UI calls it only at start, on window focus
   * and after user actions, NEVER in a timer (a poll would keep the vault
   * unlocked forever).
   */
  currentVault(): Promise<CurrentVault>
  listProjects(): Promise<ProjectDTO[]>
  createProject(name: string): Promise<ProjectDTO>
  deleteProject(projectId: string): Promise<void>
  listEnvironments(projectId: string): Promise<EnvDTO[]>
  createEnvironment(projectId: string, name: string): Promise<EnvDTO>
  deleteEnvironment(envId: string): Promise<void>
  /** Metadata only, never values. */
  listSecrets(envId: string): Promise<SecretDTO[]>
  /** Empty type keeps the existing type (text for a new secret). */
  saveSecret(envId: string, name: string, value: string, type: SecretType | ''): Promise<void>
  deleteSecret(envId: string, name: string): Promise<void>
  /** The ONLY call that returns a secret value. */
  revealSecret(envId: string, name: string): Promise<string>
  /** Go writes the clipboard; the value never reaches JS. */
  copySecret(envId: string, name: string): Promise<CopyResult>
  /** "" when the user cancelled. */
  pickEnvFile(): Promise<string>
  previewImport(envId: string, path: string): Promise<ImportPreview>
  applyImport(envId: string, path: string, strategy: ImportStrategy): Promise<void>
}

async function call<T>(f: () => Promise<T>): Promise<T> {
  try {
    return await f()
  } catch (e) {
    throw parseApiError(e)
  }
}

/** The Api over the generated Wails bindings. Results are returned untouched;
 *  the casts only narrow Go strings to our literal types. */
export function createRealApi(): Api {
  return {
    listVaults: () => call(() => B.ListVaults()),
    createVault: (name, password) => call(() => B.CreateVault(name, password)),
    unlock: (vaultId, password) => call(() => B.Unlock(vaultId, password)),
    lock: () => call(() => B.Lock()),
    selectVault: (vaultId) => call(() => B.SelectVault(vaultId)),
    currentVault: () => call(() => B.CurrentVault()),
    listProjects: () => call(() => B.ListProjects()),
    createProject: (name) => call(() => B.CreateProject(name)),
    deleteProject: (projectId) => call(() => B.DeleteProject(projectId)),
    listEnvironments: (projectId) => call(() => B.ListEnvironments(projectId)),
    createEnvironment: (projectId, name) => call(() => B.CreateEnvironment(projectId, name)),
    deleteEnvironment: (envId) => call(() => B.DeleteEnvironment(envId)),
    listSecrets: (envId) => call(() => B.ListSecrets(envId) as Promise<SecretDTO[]>),
    saveSecret: (envId, name, value, type) => call(() => B.SaveSecret(envId, name, value, type)),
    deleteSecret: (envId, name) => call(() => B.DeleteSecret(envId, name)),
    revealSecret: (envId, name) => call(() => B.RevealSecret(envId, name)),
    copySecret: (envId, name) => call(() => B.CopySecret(envId, name)),
    pickEnvFile: () => call(() => B.PickEnvFile()),
    previewImport: (envId, path) => call(() => B.PreviewImport(envId, path)),
    applyImport: (envId, path, strategy) => call(() => B.ApplyImport(envId, path, strategy)),
  }
}

/** True unless the Wails bindings are present; VITE_MOCK_API=1 forces the mock. */
export function shouldUseMock(win: unknown, mockFlag: string | undefined): boolean {
  if (mockFlag === '1') return true
  const go = (win as { go?: { desktop?: { API?: unknown } } } | undefined)?.go
  return !go?.desktop?.API
}

/** Chosen once at module load. Wails v2 defines window.go synchronously in
 *  /wails/runtime.js, which runs before the deferred module script. */
export const api: Api = shouldUseMock(globalThis.window, import.meta.env.VITE_MOCK_API)
  ? createMockApi()
  : createRealApi()

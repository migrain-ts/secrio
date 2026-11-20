// In-memory implementation of the same Api as the Go facade, for Vitest and
// for the browser dev server (no Wails, or VITE_MOCK_API=1). It mirrors the
// facade's rules and error codes (internal/desktop/*.go). All values are fake.
import type { Api } from '@/api/client'
import { ApiError } from '@/api/errors'
import {
  type CopyResult,
  type CurrentVault,
  type EnvDTO,
  type ErrorCode,
  type ImportPreview,
  type ProjectDTO,
  SECRET_TYPES,
  type SecretDTO,
  type SecretType,
  type VaultDTO,
} from '@/api/types'

export interface MockOptions {
  /** The seeded vault starts locked. */
  locked?: boolean
  /** Start with no vault at all (first run). */
  noVault?: boolean
  /** Password of the seeded vault (default "correct horse"). */
  password?: string
  /** false: the clipboard cannot be read back, so copies are not auto-cleared. */
  clipboardReadable?: boolean
  /** What pickEnvFile returns; "" = the user cancelled. */
  pickResult?: string
  /** Delay of every call, to emulate async work (default 0). */
  latencyMs?: number
}

/** Test-only hooks, deliberately outside the Api surface. */
export interface MockTestHooks {
  /** The session idle/ttl expired: the current vault becomes locked. */
  expireSession(): void
  /** `secrio lock` from the CLI: same effect, shared session. */
  lockedByCli(): void
  /** Replaces the fake file at path (null: the file cannot be read). */
  setFileContent(path: string, entries: Record<string, string> | null): void
  /** What the next pickEnvFile calls return ("" = the user cancels). */
  setPickResult(path: string): void
  /** What the mock clipboard holds ("" when empty). */
  clipboardText(): string
}

export type MockApi = Api & { readonly __test: MockTestHooks }

/** Unique string that assets.test.ts (pnpm test:dist) looks for: it must never
 *  appear in a production bundle. Set on every mock instance so it is not
 *  tree-shaken out of a bundle that does contain the mock. */
export const MOCK_MARKER = '__SECRIO_MOCK_BUNDLE__'
export const MOCK_PASSWORD = 'correct horse'
export const MOCK_IMPORT_PATH = '/Users/dev/hotenb/.env'

const MAX_VALUE_BYTES = 1 << 20
const SECRET_NAME_RE = /^[A-Za-z_][A-Za-z0-9_]*$/
const VAULT_NAME_RE = /^[A-Za-z0-9._-]{1,64}$/
const CONTROL_RE = /\p{Cc}/u // Go unicode.IsControl

interface Secret {
  value: string
  type: SecretType
  description: string
  createdAt: string
  updatedAt: string
}
interface Env {
  id: string
  name: string
  secrets: Map<string, Secret>
}
interface Project {
  id: string
  name: string
  envs: Env[]
}
interface Vault {
  id: string
  name: string
  password: string
  projects: Project[]
}

const fail = (code: ErrorCode, message: string): never => {
  throw new ApiError(code, message)
}
const now = () => new Date().toISOString().replace(/\.\d{3}Z$/, 'Z')
const byteLength = (s: string) => new TextEncoder().encode(s).length
const cmp = (a: string, b: string) => (a < b ? -1 : a > b ? 1 : 0)
// lessName in internal/desktop/structure.go: case-insensitive, then exact.
const cmpName = (a: string, b: string) => cmp(a.toLowerCase(), b.toLowerCase()) || cmp(a, b)

function validName(name: string): void {
  if (name.trim() === '' || CONTROL_RE.test(name)) fail('invalid_name', 'invalid name')
}
function validSecretName(name: string): void {
  if (!SECRET_NAME_RE.test(name)) fail('invalid_name', 'invalid name')
}
function validVaultName(name: string): boolean {
  if (!VAULT_NAME_RE.test(name) || name.startsWith('.') || name.startsWith('-') || name.includes('..')) {
    return false
  }
  const base = name.toLowerCase().split('.')[0]
  return !(['con', 'prn', 'aux', 'nul'].includes(base) || /^(com|lpt)[1-9]$/.test(base))
}

// --- seed ------------------------------------------------------------------

type SeedSecret = [name: string, type: SecretType, value: string, description?: string]

const T0 = '2026-10-02T09:14:00Z'
const T1 = '2026-11-12T16:40:00Z'

const CORE: SeedSecret[] = [
  ['DATABASE_URL', 'url', 'postgres://app:••••@db:5432/app', 'Primary Postgres'],
  ['JWT_SECRET', 'password', 'fake-jwt-signing-secret-0000'],
  ['API_KEY', 'api_key', 'fake-api-key-1111-2222'],
  ['STRIPE_KEY', 'api_key', 'fake-stripe-test-key-3333', 'Test mode only'],
  ['REDIS_URL', 'url', 'redis://cache:6379/0'],
]
const SEED: [project: string, env: string, secrets: SeedSecret[]][] = [
  [
    'hotenb',
    'Development',
    [...CORE, ['SENTRY_DSN', 'url', 'https://fake@sentry.example/1'], ['GITHUB_TOKEN', 'token', 'fake-gh-token-4444']],
  ],
  [
    'hotenb',
    'Staging',
    [
      ['SENTRY_DSN', 'url', 'https://fake@sentry.example/2', 'Error tracking'],
      ['LOG_LEVEL', 'text', 'debug'],
    ],
  ],
  ['hotenb', 'Production', [...CORE, ['SENTRY_DSN', 'url', 'https://fake@sentry.example/3']]],
  [
    'portfolio',
    'Development',
    [
      ['CONTACT_API_KEY', 'api_key', 'fake-mail-key-5555'],
      ['SITE_URL', 'url', 'http://localhost:5173'],
    ],
  ],
  [
    'portfolio',
    'Production',
    [
      ['CONTACT_API_KEY', 'api_key', 'fake-mail-key-6666'],
      ['SITE_URL', 'url', 'https://portfolio.example'],
      ['ANALYTICS_ID', 'text', 'fake-analytics-7777'],
    ],
  ],
  [
    'infra',
    'Production',
    [
      ['TF_API_TOKEN', 'token', 'fake-terraform-token-8888', 'Terraform Cloud'],
      ['DEPLOY_SSH_KEY', 'private_key', 'fake-ssh-key-material-not-a-real-key'],
      ['TLS_CERT', 'certificate', 'fake-certificate-body'],
      ['CLOUD_CONFIG', 'json', '{"region":"eu-west-1","profile":"fake"}'],
    ],
  ],
]
const SEED_FILES: Record<string, Record<string, string>> = {
  [MOCK_IMPORT_PATH]: {
    DATABASE_URL: 'postgres://app:••••@staging-db:5432/app',
    REDIS_URL: 'redis://staging-cache:6379/0',
    SENTRY_DSN: 'https://fake@sentry.example/9',
    FEATURE_FLAGS: 'beta-dashboard',
  },
}

const slug = (s: string) => s.toLowerCase().replace(/[^a-z0-9]+/g, '-')

function seedProjects(): Project[] {
  const projects: Project[] = []
  for (const [pName, eName, secrets] of SEED) {
    let p = projects.find((x) => x.name === pName)
    if (!p) projects.push((p = { id: `p-${slug(pName)}`, name: pName, envs: [] }))
    const env: Env = { id: `e-${slug(pName)}-${slug(eName)}`, name: eName, secrets: new Map() }
    for (const [name, type, value, description = ''] of secrets) {
      env.secrets.set(name, { value, type, description, createdAt: T0, updatedAt: T1 })
    }
    p.envs.push(env)
  }
  return projects
}

// --- the mock ----------------------------------------------------------------

export function createMockApi(opts: MockOptions = {}): MockApi {
  const latency = opts.latencyMs ?? 0
  const clipboardReadable = opts.clipboardReadable ?? true
  let pickResult = opts.pickResult ?? MOCK_IMPORT_PATH

  const vaults: Vault[] = opts.noVault
    ? []
    : [{ id: 'v-work', name: 'work', password: opts.password ?? MOCK_PASSWORD, projects: seedProjects() }]
  let current = opts.noVault ? '' : 'v-work'
  const unlocked = new Set<string>(opts.noVault || opts.locked ? [] : ['v-work'])
  const files = new Map<string, Record<string, string> | null>(Object.entries(SEED_FILES))
  let picked = new Set<string>()
  let previews = new Map<string, string>() // envId \0 path -> plan key
  let clipboard = ''
  let ourCopy: string | null = null
  let nextId = 1

  /** Every call is async and optionally delayed; a throw becomes a rejection. */
  const run = async <T>(f: () => T): Promise<T> => {
    if (latency > 0) await new Promise((r) => setTimeout(r, latency))
    return f()
  }

  const findVault = (id: string) => vaults.find((v) => v.id === id)
  /** resolveCurrent: the selection, else the only vault. */
  const resolveCurrent = (): Vault | undefined => {
    const v = findVault(current)
    if (v) return v
    if (vaults.length === 1) {
      current = vaults[0].id
      return vaults[0]
    }
    current = ''
    return undefined
  }
  /** withVault: no_vault, then locked. */
  const open = (): Vault => {
    const v = resolveCurrent() ?? fail('no_vault', 'no vault selected')
    if (!unlocked.has(v.id)) fail('locked', 'vault is locked')
    return v
  }
  const findEnv = (v: Vault, envId: string): Env => {
    for (const p of v.projects) {
      const e = p.envs.find((x) => x.id === envId)
      if (e) return e
    }
    return fail('not_found', 'not found')
  }
  const findSecret = (env: Env, name: string): Secret => env.secrets.get(name) ?? fail('not_found', 'not found')
  const envDTO = (e: Env): EnvDTO => ({ id: e.id, name: e.name, secretCount: e.secrets.size })
  const sortedEnvs = (p: Project) => [...p.envs].sort((a, b) => cmpName(a.name, b.name))
  const clearOurClipboard = () => {
    if (ourCopy !== null && clipboard === ourCopy) clipboard = ''
    ourCopy = null
  }
  const plan = (env: Env, path: string): ImportPreview => {
    const entries = files.get(path) ?? fail('not_found', 'the file could not be read')
    const names = Object.keys(entries)
    if (names.some((n) => !SECRET_NAME_RE.test(n))) fail('invalid_value', 'the file contains an invalid entry')
    return {
      new: names.filter((n) => !env.secrets.has(n)).sort(cmp),
      duplicate: names.filter((n) => env.secrets.has(n)).sort(cmp),
    }
  }
  const checkPicked = (path: string) => {
    if (path === '' || !picked.has(path)) fail('invalid_value', 'choose the file with the file picker first')
  }

  const api: Api = {
    listVaults: () =>
      run(() => vaults.map((v): VaultDTO => ({ id: v.id, name: v.name })).sort((a, b) => cmp(a.name, b.name))),

    createVault: (name, password) =>
      run(() => {
        if (!validVaultName(name)) fail('invalid_name', 'invalid name')
        if (password === '') fail('invalid_value', 'password must not be empty')
        if (vaults.some((v) => v.name.toLowerCase() === name.toLowerCase())) fail('duplicate', 'already exists')
        const v: Vault = { id: `v-${nextId++}`, name, password, projects: [] }
        vaults.push(v)
        current = v.id
        unlocked.add(v.id)
        return { id: v.id, name: v.name }
      }),

    unlock: (vaultId, password) =>
      run(() => {
        const v = findVault(vaultId) ?? fail('not_found', 'not found')
        if (password !== v.password) fail('invalid_password', 'invalid password')
        unlocked.add(v.id)
        current = v.id
      }),

    lock: () =>
      run(() => {
        clearOurClipboard()
        picked = new Set()
        previews = new Map()
        const v = resolveCurrent() ?? fail('no_vault', 'no vault selected')
        unlocked.delete(v.id)
      }),

    selectVault: (vaultId) =>
      run(() => {
        current = (findVault(vaultId) ?? fail('not_found', 'not found')).id
      }),

    currentVault: () =>
      run((): CurrentVault => {
        const v = resolveCurrent()
        if (!v) return { vault: { id: '', name: '' }, unlocked: false }
        return { vault: { id: v.id, name: v.name }, unlocked: unlocked.has(v.id) }
      }),

    listProjects: () =>
      run(() =>
        open()
          .projects.map(
            (p): ProjectDTO => ({
              id: p.id,
              name: p.name,
              envCount: p.envs.length,
              secretCount: p.envs.reduce((n, e) => n + e.secrets.size, 0),
            }),
          )
          .sort((a, b) => cmpName(a.name, b.name)),
      ),

    createProject: (name) =>
      run(() => {
        const v = open()
        validName(name)
        if (v.projects.some((p) => p.name === name)) fail('duplicate', 'already exists')
        const p: Project = { id: `p-${nextId++}`, name, envs: [] }
        v.projects.push(p)
        return { id: p.id, name: p.name, envCount: 0, secretCount: 0 }
      }),

    deleteProject: (projectId) =>
      run(() => {
        const v = open()
        const i = v.projects.findIndex((p) => p.id === projectId)
        if (i < 0) fail('not_found', 'not found')
        v.projects.splice(i, 1)
      }),

    listEnvironments: (projectId) =>
      run(() => {
        const p = open().projects.find((x) => x.id === projectId) ?? fail('not_found', 'not found')
        return sortedEnvs(p).map(envDTO)
      }),

    createEnvironment: (projectId, name) =>
      run(() => {
        const v = open()
        validName(name)
        const p = v.projects.find((x) => x.id === projectId) ?? fail('not_found', 'not found')
        if (p.envs.some((e) => e.name === name)) fail('duplicate', 'already exists')
        const e: Env = { id: `e-${nextId++}`, name, secrets: new Map() }
        p.envs.push(e)
        return envDTO(e)
      }),

    deleteEnvironment: (envId) =>
      run(() => {
        const v = open()
        for (const p of v.projects) {
          const i = p.envs.findIndex((e) => e.id === envId)
          if (i >= 0) {
            p.envs.splice(i, 1)
            return
          }
        }
        fail('not_found', 'not found')
      }),

    listSecrets: (envId) =>
      run(() =>
        [...findEnv(open(), envId).secrets]
          .map(
            ([name, s]): SecretDTO => ({
              name,
              type: s.type,
              description: s.description,
              createdAt: s.createdAt,
              updatedAt: s.updatedAt,
            }),
          )
          .sort((a, b) => cmp(a.name, b.name)),
      ),

    saveSecret: (envId, name, value, type) =>
      run(() => {
        validSecretName(name)
        if (value === '' || value.includes('\0') || byteLength(value) > MAX_VALUE_BYTES) {
          fail('invalid_value', 'invalid value')
        }
        if (type !== '' && !(SECRET_TYPES as readonly string[]).includes(type)) fail('invalid_value', 'invalid value')
        const env = findEnv(open(), envId)
        const old = env.secrets.get(name)
        const t = now()
        env.secrets.set(name, {
          value,
          type: type || old?.type || 'text',
          description: old?.description ?? '',
          createdAt: old?.createdAt ?? t,
          updatedAt: t,
        })
      }),

    deleteSecret: (envId, name) =>
      run(() => {
        validSecretName(name)
        const env = findEnv(open(), envId)
        findSecret(env, name)
        env.secrets.delete(name)
      }),

    revealSecret: (envId, name) =>
      run(() => {
        validSecretName(name)
        return findSecret(findEnv(open(), envId), name).value
      }),

    copySecret: (envId, name) =>
      run((): CopyResult => {
        validSecretName(name)
        clipboard = findSecret(findEnv(open(), envId), name).value
        if (!clipboardReadable) {
          ourCopy = null
          return { clearInSeconds: 0 }
        }
        ourCopy = clipboard
        return { clearInSeconds: 30 }
      }),

    pickEnvFile: () =>
      run(() => {
        if (pickResult !== '') picked.add(pickResult)
        return pickResult
      }),

    previewImport: (envId, path) =>
      run(() => {
        checkPicked(path)
        const pv = plan(findEnv(open(), envId), path)
        previews.set(`${envId}\0${path}`, JSON.stringify(pv))
        return pv
      }),

    applyImport: (envId, path, strategy) =>
      run(() => {
        if (strategy !== 'skip' && strategy !== 'overwrite') {
          fail('invalid_value', 'strategy must be skip or overwrite')
        }
        checkPicked(path)
        const key = `${envId}\0${path}`
        const want = previews.get(key) ?? fail('conflict', 'preview the import first')
        const env = findEnv(open(), envId)
        const pv = plan(env, path)
        if (JSON.stringify(pv) !== want) {
          previews.delete(key)
          fail('conflict', 'the file or the environment changed; preview again')
        }
        const entries = files.get(path)!
        const t = now()
        for (const n of pv.new) {
          env.secrets.set(n, { value: entries[n], type: 'text', description: '', createdAt: t, updatedAt: t })
        }
        if (strategy === 'overwrite') {
          for (const n of pv.duplicate) {
            const s = env.secrets.get(n)!
            env.secrets.set(n, { ...s, value: entries[n], updatedAt: t }) // the value only
          }
        }
        previews.delete(key)
      }),
  }

  const __test: MockTestHooks = {
    expireSession: () => {
      if (current) unlocked.delete(current)
    },
    lockedByCli: () => {
      if (current) unlocked.delete(current)
    },
    setFileContent: (path, entries) => {
      files.set(path, entries === null ? null : { ...entries })
    },
    setPickResult: (path) => {
      pickResult = path
    },
    clipboardText: () => clipboard,
  }

  Object.defineProperty(api, MOCK_MARKER, { value: true }) // non-enumerable
  return Object.assign(api, { __test })
}

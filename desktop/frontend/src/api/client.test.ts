import { afterEach, describe, expect, test, vi } from 'vitest'
import {
  API_METHODS,
  type Api,
  createLazyMockApi,
  createRealApi,
  createUnavailableApi,
} from '@/api/client'
import { ApiError } from '@/api/errors'

type Fn = (...args: unknown[]) => Promise<unknown>

// [Api method, Go binding, distinct arguments, value the binding resolves with]
const cases: [keyof Api, string, unknown[], unknown][] = [
  ['listVaults', 'ListVaults', [], [{ id: 'v1', name: 'work' }]],
  ['createVault', 'CreateVault', ['work', 'pw-1'], { id: 'v1', name: 'work' }],
  ['unlock', 'Unlock', ['v1', 'pw-2'], undefined],
  ['lock', 'Lock', [], undefined],
  ['selectVault', 'SelectVault', ['v2'], undefined],
  ['currentVault', 'CurrentVault', [], { vault: { id: 'v1', name: 'work' }, unlocked: true }],
  ['listProjects', 'ListProjects', [], [{ id: 'p1', name: 'a', envCount: 1, secretCount: 2 }]],
  ['createProject', 'CreateProject', ['proj-a'], { id: 'p1', name: 'proj-a', envCount: 0, secretCount: 0 }],
  ['deleteProject', 'DeleteProject', ['p9'], undefined],
  ['listEnvironments', 'ListEnvironments', ['p3'], [{ id: 'e1', name: 'Dev', secretCount: 0 }]],
  ['createEnvironment', 'CreateEnvironment', ['p4', 'Staging'], { id: 'e2', name: 'Staging', secretCount: 0 }],
  ['deleteEnvironment', 'DeleteEnvironment', ['e7'], undefined],
  [
    'listSecrets',
    'ListSecrets',
    ['e8'],
    [{ name: 'A', type: 'text', description: '', createdAt: 't0', updatedAt: 't1' }],
  ],
  ['saveSecret', 'SaveSecret', ['e9', 'NAME_1', 'value-1', 'token'], undefined],
  ['deleteSecret', 'DeleteSecret', ['e10', 'NAME_2'], undefined],
  ['revealSecret', 'RevealSecret', ['e11', 'NAME_3'], 'plain-value'],
  ['copySecret', 'CopySecret', ['e12', 'NAME_4'], { clearInSeconds: 30 }],
  ['pickEnvFile', 'PickEnvFile', [], '/tmp/a.env'],
  ['previewImport', 'PreviewImport', ['e13', '/tmp/b.env'], { new: ['X'], duplicate: ['Y'] }],
  ['applyImport', 'ApplyImport', ['e14', '/tmp/c.env', 'overwrite'], undefined],
]

function stubBindings(impl: (name: string) => Fn) {
  const API: Record<string, Fn> = {}
  for (const [, goName] of cases) API[goName] = vi.fn(impl(goName))
  vi.stubGlobal('go', { desktop: { API } })
  return API
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.unstubAllEnvs()
  vi.resetModules()
})

describe('createRealApi', () => {
  test('covers the 20 bound methods', () => {
    expect(cases).toHaveLength(20)
    expect(Object.keys(createRealApi()).sort()).toEqual(cases.map((c) => c[0]).sort())
  })

  test.each(cases)('%s passes its arguments in order and returns the value untouched', async (method, goName, args, ret) => {
    const API = stubBindings(() => async () => ret)
    const real = createRealApi() as unknown as Record<string, Fn>
    const got = await real[method](...args)
    expect(API[goName]).toHaveBeenCalledTimes(1)
    expect(API[goName]).toHaveBeenCalledWith(...args)
    expect(got).toBe(ret)
  })

  test.each(cases)('%s maps a rejected Error(JSON) to ApiError', async (method, _goName, args) => {
    stubBindings(() => async () => {
      throw new Error('{"code":"locked","message":"vault is locked"}')
    })
    const real = createRealApi() as unknown as Record<string, Fn>
    const err = await real[method](...args).catch((e: unknown) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect((err as ApiError).code).toBe('locked')
  })

  test('maps each code it receives', async () => {
    stubBindings(() => async () => {
      throw new Error('{"code":"invalid_password","message":"invalid password"}')
    })
    await expect(createRealApi().unlock('v1', 'x')).rejects.toMatchObject({ code: 'invalid_password' })
  })

  test('unknown rejection text never reaches the caller', async () => {
    stubBindings(() => async () => {
      throw new Error('panic: secret=hunter2')
    })
    const err = (await createRealApi()
      .revealSecret('e', 'N')
      .catch((e: unknown) => e)) as ApiError
    expect(err.code).toBe('internal')
    expect(err.message).toBe('unexpected error')
  })

  test('a missing window.go becomes an internal ApiError, not a TypeError', async () => {
    vi.stubGlobal('go', undefined)
    await expect(createRealApi().listVaults()).rejects.toMatchObject({ code: 'internal' })
  })
})

describe('api selection', () => {
  test('API_METHODS lists exactly the Api methods', () => {
    expect([...API_METHODS].sort()).toEqual(Object.keys(createRealApi()).sort())
  })

  test('module picks the real client when window.go.desktop.API exists', async () => {
    const API = stubBindings(() => async () => [])
    const { api, backendStatus } = await import('@/api/client')
    expect(backendStatus).toBe('real')
    await api.listVaults()
    expect(API.ListVaults).toHaveBeenCalledTimes(1)
  })

  test('module picks the (lazy) mock without window.go in test mode', async () => {
    vi.stubGlobal('go', undefined)
    const { api, backendStatus } = await import('@/api/client')
    expect(backendStatus).toBe('mock')
    expect(await api.listVaults()).toEqual([{ id: 'v-work', name: 'work' }])
  })

  test('the lazy mock keeps one store across calls', async () => {
    vi.stubGlobal('go', undefined)
    const { api } = await import('@/api/client')
    await api.createProject('lazy-one')
    expect((await api.listProjects()).map((p) => p.name)).toContain('lazy-one')
  })

  test('VITE_MOCK_API=1 forces the mock even inside Wails (dev/test only)', async () => {
    const API = stubBindings(() => async () => [])
    vi.stubEnv('VITE_MOCK_API', '1')
    const { api, backendStatus } = await import('@/api/client')
    expect(backendStatus).toBe('mock')
    expect(await api.listVaults()).toEqual([{ id: 'v-work', name: 'work' }])
    expect(API.ListVaults).not.toHaveBeenCalled()
  })

  test('production without bindings is unavailable, never the mock', async () => {
    vi.stubGlobal('go', undefined)
    vi.stubEnv('DEV', false)
    vi.stubEnv('PROD', true)
    vi.stubEnv('MODE', 'production')
    vi.stubEnv('VITE_MOCK_API', '1')
    const { api, backendStatus } = await import('@/api/client')
    expect(backendStatus).toBe('unavailable')
    await expect(api.listVaults()).rejects.toMatchObject({ code: 'internal', message: 'backend unavailable' })
  })

  test('production with bindings is real, even with VITE_MOCK_API', async () => {
    const API = stubBindings(() => async () => [])
    vi.stubEnv('DEV', false)
    vi.stubEnv('PROD', true)
    vi.stubEnv('MODE', 'production')
    vi.stubEnv('VITE_MOCK_API', '1')
    const { api, backendStatus } = await import('@/api/client')
    expect(backendStatus).toBe('real')
    await api.listVaults()
    expect(API.ListVaults).toHaveBeenCalledTimes(1)
  })
})

describe('createUnavailableApi', () => {
  test.each(cases)('%s rejects with internal "backend unavailable"', async (method, _goName, args) => {
    const un = createUnavailableApi() as unknown as Record<string, Fn>
    const err = await un[method](...args).catch((e: unknown) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect(err).toMatchObject({ code: 'internal', message: 'backend unavailable' })
  })

  test('has the 20 methods', () => {
    expect(Object.keys(createUnavailableApi()).sort()).toEqual(cases.map((c) => c[0]).sort())
  })
})

describe('createLazyMockApi', () => {
  test('forwards every method with its arguments to the loaded api', async () => {
    const calls: [string, unknown[]][] = []
    const target = Object.fromEntries(
      cases.map(([m]) => [m, async (...a: unknown[]) => (calls.push([m, a]), `r-${m}`)]),
    ) as unknown as Api
    let loads = 0
    const lazy = createLazyMockApi(async () => (loads++, target)) as unknown as Record<string, Fn>
    for (const [m, , args] of cases) expect(await lazy[m](...args)).toBe(`r-${m}`)
    expect(calls).toEqual(cases.map(([m, , args]) => [m, args]))
    expect(loads).toBe(20) // the loader is called per call; it caches the module itself
  })

  test('a loader failure becomes an ApiError', async () => {
    const lazy = createLazyMockApi(async () => {
      throw new Error('chunk failed')
    })
    await expect(lazy.listVaults()).rejects.toMatchObject({ code: 'internal' })
  })
})

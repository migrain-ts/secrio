import { afterEach, describe, expect, test, vi } from 'vitest'
import { type Api, createRealApi, shouldUseMock } from '@/api/client'
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
  test('shouldUseMock', () => {
    const withGo = { go: { desktop: { API: {} } } }
    expect(shouldUseMock(withGo, undefined)).toBe(false)
    expect(shouldUseMock(withGo, '1')).toBe(true)
    expect(shouldUseMock({}, undefined)).toBe(true)
    expect(shouldUseMock({ go: {} }, undefined)).toBe(true)
    expect(shouldUseMock({ go: { desktop: {} } }, undefined)).toBe(true)
    expect(shouldUseMock(undefined, undefined)).toBe(true)
    expect(shouldUseMock(withGo, '0')).toBe(false)
  })

  test('module picks the real client when window.go.desktop.API exists', async () => {
    const API = stubBindings(() => async () => [])
    const { api } = await import('@/api/client')
    await api.listVaults()
    expect(API.ListVaults).toHaveBeenCalledTimes(1)
  })

  test('module picks the mock without window.go', async () => {
    vi.stubGlobal('go', undefined)
    const { api } = await import('@/api/client')
    expect(await api.listVaults()).toEqual([{ id: 'v-work', name: 'work' }])
  })

  test('VITE_MOCK_API=1 forces the mock even inside Wails', async () => {
    const API = stubBindings(() => async () => [])
    vi.stubEnv('VITE_MOCK_API', '1')
    const { api } = await import('@/api/client')
    expect(await api.listVaults()).toEqual([{ id: 'v-work', name: 'work' }])
    expect(API.ListVaults).not.toHaveBeenCalled()
  })
})

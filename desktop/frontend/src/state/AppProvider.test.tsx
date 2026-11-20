import { act, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import { API_METHODS, type Api } from '@/api/client'
import { ApiError } from '@/api/errors'
import { createMockApi, MOCK_PASSWORD, type MockApi } from '@/api/mock'
import { AppProvider, useApp } from '@/state/AppProvider'

/** Shows the state as text and exposes the context to the test. */
let ctx: ReturnType<typeof useApp>
function Probe() {
  ctx = useApp()
  return <output data-testid="screen">{JSON.stringify(ctx.screen)}</output>
}
const shown = () => JSON.parse(screen.getByTestId('screen').textContent ?? 'null') as unknown

function mount(api: Api, backendStatus: 'real' | 'mock' | 'unavailable' = 'mock') {
  return render(
    <AppProvider api={api} backendStatus={backendStatus}>
      <Probe />
    </AppProvider>,
  )
}

/** A mock whose methods are all spies. */
function spied(m: MockApi) {
  for (const k of API_METHODS) vi.spyOn(m, k)
  return m
}

afterEach(() => {
  vi.useRealTimers()
})

describe('boot', () => {
  test('an open session lands on projects', async () => {
    mount(createMockApi())
    await waitFor(() => expect(shown()).toEqual({ kind: 'projects' }))
    expect(ctx.vault).toEqual({ id: 'v-work', name: 'work' })
  })

  test('a locked last vault lands on unlock with it preselected', async () => {
    mount(createMockApi({ locked: true }))
    await waitFor(() => expect(shown()).toEqual({ kind: 'unlock', vaultId: 'v-work' }))
  })

  test('no vault at all lands on create-vault', async () => {
    mount(createMockApi({ noVault: true }))
    await waitFor(() => expect(shown()).toEqual({ kind: 'create-vault' }))
  })

  test('no current vault but some listed lands on unlock without preselection', async () => {
    const m = createMockApi({ noVault: true })
    await m.createVault('alpha', 'pw')
    await m.createVault('beta', 'pw')
    await m.lock()
    // no current selection: the mock has 2 vaults and we clear its choice
    const api: Api = { ...m, currentVault: async () => ({ vault: { id: '', name: '' }, unlocked: false }) }
    mount(api)
    await waitFor(() => expect(shown()).toEqual({ kind: 'unlock', vaultId: null }))
  })

  test('backend unavailable shows fatal and makes no api call at all', async () => {
    const m = spied(createMockApi())
    mount(m, 'unavailable')
    await waitFor(() => expect(shown()).toEqual({ kind: 'fatal' }))
    const called = API_METHODS.filter((k) => vi.mocked(m[k]).mock.calls.length > 0)
    expect(called).toEqual([])
  })
})

describe('guarded api', () => {
  test('a locked error dispatches LOCKED with the current screen and is rethrown', async () => {
    const m = createMockApi()
    mount(m)
    await waitFor(() => expect(shown()).toEqual({ kind: 'projects' }))
    act(() => ctx.dispatch({ type: 'OPEN_PROJECT', projectId: 'p-x', envId: 'e-y' }))
    m.__test.lockedByCli()
    let caught: unknown
    await act(async () => {
      await ctx.api.listProjects().catch((e: unknown) => (caught = e))
    })
    expect(caught).toBeInstanceOf(ApiError)
    expect((caught as ApiError).code).toBe('locked')
    expect(shown()).toEqual({ kind: 'unlock', vaultId: 'v-work', notice: 'locked' })
    // unlocking again restores the project
    await act(async () => {
      await ctx.api.unlock('v-work', MOCK_PASSWORD)
      ctx.dispatch({ type: 'UNLOCKED', vaultId: 'v-work' })
    })
    expect(shown()).toEqual({ kind: 'project', projectId: 'p-x', envId: 'e-y' })
  })

  test('other errors are rethrown and change nothing', async () => {
    mount(createMockApi())
    await waitFor(() => expect(shown()).toEqual({ kind: 'projects' }))
    let caught: unknown
    await act(async () => {
      await ctx.api.revealSecret('nope', 'X').catch((e: unknown) => (caught = e))
    })
    expect((caught as ApiError).code).toBe('not_found')
    expect(shown()).toEqual({ kind: 'projects' })
  })

  test('success values pass through untouched', async () => {
    mount(createMockApi())
    await waitFor(() => expect(shown()).toEqual({ kind: 'projects' }))
    const ps = await ctx.api.listProjects()
    expect(ps.map((p) => p.name)).toEqual(['hotenb', 'infra', 'portfolio'])
  })

  test('no_vault goes to unlock when vaults exist, else to create-vault', async () => {
    const m = createMockApi()
    const noVault = () => Promise.reject(new ApiError('no_vault', 'no vault selected'))
    mount({ ...m, listProjects: noVault })
    await waitFor(() => expect(shown()).toEqual({ kind: 'projects' }))
    await act(async () => {
      await ctx.api.listProjects().catch(() => {})
    })
    await waitFor(() => expect(shown()).toEqual({ kind: 'unlock', vaultId: null }))
  })

  test('no_vault with no vault listed goes to create-vault', async () => {
    const m = createMockApi()
    const noVault = () => Promise.reject(new ApiError('no_vault', 'no vault selected'))
    let listed = true
    mount({ ...m, listProjects: noVault, listVaults: async () => (listed ? m.listVaults() : []) })
    await waitFor(() => expect(shown()).toEqual({ kind: 'projects' }))
    listed = false
    await act(async () => {
      await ctx.api.listProjects().catch(() => {})
    })
    await waitFor(() => expect(shown()).toEqual({ kind: 'create-vault' }))
  })
})

describe('focus check', () => {
  const focus = () => act(() => void window.dispatchEvent(new Event('focus')))

  test('focus on a data screen with a closed session goes to unlock', async () => {
    const m = createMockApi()
    mount(m)
    await waitFor(() => expect(shown()).toEqual({ kind: 'projects' }))
    m.__test.expireSession()
    focus()
    await waitFor(() => expect(shown()).toEqual({ kind: 'unlock', vaultId: 'v-work', notice: 'locked' }))
  })

  test('focus events within 2 s of the last check are ignored', async () => {
    const m = spied(createMockApi())
    mount(m)
    await waitFor(() => expect(shown()).toEqual({ kind: 'projects' }))
    const base = vi.mocked(m.currentVault).mock.calls.length
    const now = vi.spyOn(Date, 'now')
    now.mockReturnValue(10_000)
    focus()
    now.mockReturnValue(11_500)
    focus()
    focus()
    expect(vi.mocked(m.currentVault).mock.calls.length).toBe(base + 1)
    now.mockReturnValue(12_001)
    focus()
    expect(vi.mocked(m.currentVault).mock.calls.length).toBe(base + 2)
    now.mockRestore()
  })

  test('no focus check outside data screens', async () => {
    const m = spied(createMockApi({ locked: true }))
    mount(m)
    await waitFor(() => expect(shown()).toEqual({ kind: 'unlock', vaultId: 'v-work' }))
    const base = vi.mocked(m.currentVault).mock.calls.length
    focus()
    expect(vi.mocked(m.currentVault).mock.calls.length).toBe(base)
  })

  test('never polls currentVault: 3 hours of fake time add no call', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    const m = spied(createMockApi())
    mount(m)
    await waitFor(() => expect(shown()).toEqual({ kind: 'projects' }))
    const base = vi.mocked(m.currentVault).mock.calls.length
    await act(async () => {
      await vi.advanceTimersByTimeAsync(3 * 60 * 60 * 1000)
    })
    expect(vi.mocked(m.currentVault).mock.calls.length).toBe(base)
    expect(vi.getTimerCount()).toBe(0)
  })
})

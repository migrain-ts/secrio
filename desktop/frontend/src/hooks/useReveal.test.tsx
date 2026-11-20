import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'
import type { Api } from '@/api/client'
import { ApiError } from '@/api/errors'
import { createMockApi } from '@/api/mock'
import { REVEAL_MS, useReveal } from '@/hooks/useReveal'

const ENV = 'e-hotenb-development'
const OTHER_ENV = 'e-hotenb-staging'
const VALUE = 'fake-jwt-signing-secret-0000' // JWT_SECRET in the mock seed

function setup(api: Api = createMockApi(), onNotFound = vi.fn()) {
  const r = renderHook(
    ({ envId, name }: { envId: string | null; name: string | null }) => useReveal(api, envId, name, onNotFound),
    { initialProps: { envId: ENV as string | null, name: 'JWT_SECRET' as string | null } },
  )
  const reveal = () => act(() => r.result.current.reveal())
  return { ...r, reveal, onNotFound }
}

beforeEach(() => {
  vi.useFakeTimers()
})
afterEach(() => {
  vi.useRealTimers()
})

describe('useReveal', () => {
  test('starts hidden; reveal fetches the selected value', async () => {
    const { result, reveal } = setup()
    expect(result.current.value).toBeNull()
    await reveal()
    expect(result.current.value).toBe(VALUE)
  })

  test(`hides after ${REVEAL_MS / 1000} s`, async () => {
    const { result, reveal } = setup()
    await reveal()
    act(() => vi.advanceTimersByTime(REVEAL_MS - 1))
    expect(result.current.value).toBe(VALUE)
    act(() => vi.advanceTimersByTime(1))
    expect(result.current.value).toBeNull()
  })

  test('hides when the selection changes', async () => {
    const { result, reveal, rerender } = setup()
    await reveal()
    rerender({ envId: ENV, name: 'API_KEY' })
    expect(result.current.value).toBeNull()
    // and does not come back when the selection returns
    rerender({ envId: ENV, name: 'JWT_SECRET' })
    expect(result.current.value).toBeNull()
  })

  test('hides when the env changes', async () => {
    const { result, reveal, rerender } = setup()
    await reveal()
    rerender({ envId: OTHER_ENV, name: 'JWT_SECRET' })
    expect(result.current.value).toBeNull()
  })

  test('hides when the window loses focus', async () => {
    const { result, reveal } = setup()
    await reveal()
    act(() => void window.dispatchEvent(new Event('blur')))
    expect(result.current.value).toBeNull()
  })

  test('hides when the page becomes hidden', async () => {
    const { result, reveal } = setup()
    await reveal()
    const vis = vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden')
    act(() => void document.dispatchEvent(new Event('visibilitychange')))
    expect(result.current.value).toBeNull()
    vis.mockRestore()
  })

  test('hide() hides at once (copy, edit and delete call it)', async () => {
    const { result, reveal } = setup()
    await reveal()
    act(() => result.current.hide())
    expect(result.current.value).toBeNull()
  })

  test('a reveal that resolves after the selection changed is dropped', async () => {
    let resolve!: (v: string) => void
    const m = createMockApi()
    const api: Api = { ...m, revealSecret: () => new Promise<string>((r) => (resolve = r)) }
    const { result, rerender } = setup(api)
    let p!: Promise<void>
    act(() => {
      p = result.current.reveal()
    })
    expect(result.current.pending).toBe(true)
    rerender({ envId: ENV, name: 'API_KEY' })
    await act(async () => {
      resolve(VALUE)
      await p
    })
    expect(result.current.value).toBeNull()
    expect(result.current.pending).toBe(false)
  })

  test('a failure sets a short error, never the server text; not_found asks for a refresh', async () => {
    const m = createMockApi()
    const api: Api = { ...m, revealSecret: () => Promise.reject(new ApiError('not_found', 'not found')) }
    const { result, reveal, onNotFound } = setup(api)
    await reveal()
    expect(result.current.value).toBeNull()
    expect(result.current.failed).toBe(true)
    expect(onNotFound).toHaveBeenCalledOnce()
  })

  test('the timer is cleared on unmount', async () => {
    const { reveal, unmount } = setup()
    await reveal()
    unmount()
    expect(vi.getTimerCount()).toBe(0)
  })
})

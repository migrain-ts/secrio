import { expect, test } from 'vitest'
import { hasBindings, selectApi, type BackendStatus } from '@/api/select'

type Row = [dev: boolean, mode: string, forceMock: boolean, bindings: boolean, want: BackendStatus]

// Production never selects the mock, whatever the flags.
const rows: Row[] = [
  [false, 'production', false, true, 'real'],
  [false, 'production', false, false, 'unavailable'],
  [false, 'production', true, true, 'real'],
  [false, 'production', true, false, 'unavailable'],
  [false, 'staging', false, false, 'unavailable'], // any non-dev, non-test mode is production-like
  [true, 'development', false, false, 'mock'],
  [true, 'development', false, true, 'real'],
  [true, 'development', true, true, 'mock'],
  [true, 'development', true, false, 'mock'],
  [false, 'test', false, false, 'mock'],
  [false, 'test', false, true, 'real'],
  [false, 'test', true, true, 'mock'],
]

test.each(rows)('dev=%s mode=%s forceMock=%s bindings=%s -> %s', (dev, mode, forceMock, bindings, want) => {
  expect(selectApi({ dev, mode, forceMock, hasBindings: bindings })).toBe(want)
})

test('hasBindings looks for window.go.desktop.API', () => {
  expect(hasBindings({ go: { desktop: { API: {} } } })).toBe(true)
  expect(hasBindings({ go: { desktop: {} } })).toBe(false)
  expect(hasBindings({ go: {} })).toBe(false)
  expect(hasBindings({})).toBe(false)
  expect(hasBindings(undefined)).toBe(false)
})

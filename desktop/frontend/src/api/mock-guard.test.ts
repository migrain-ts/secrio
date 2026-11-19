// @vitest-environment node
import { expect, test } from 'vitest'
import { assertNoMockInProduction } from '@/api/mock-guard'

test('production build refuses VITE_MOCK_API', () => {
  expect(() => assertNoMockInProduction('production', { VITE_MOCK_API: '1' })).toThrow(/VITE_MOCK_API/)
  expect(() => assertNoMockInProduction('production', { VITE_MOCK_API: '0' })).toThrow(/VITE_MOCK_API/)
  // any mode other than development/test builds like production
  expect(() => assertNoMockInProduction('staging', { VITE_MOCK_API: '1' })).toThrow(/VITE_MOCK_API/)
})

test('production build without the flag, and dev/test with it, are fine', () => {
  expect(() => assertNoMockInProduction('production', {})).not.toThrow()
  expect(() => assertNoMockInProduction('production', { VITE_MOCK_API: '' })).not.toThrow()
  expect(() => assertNoMockInProduction('development', { VITE_MOCK_API: '1' })).not.toThrow()
  expect(() => assertNoMockInProduction('test', { VITE_MOCK_API: '1' })).not.toThrow()
})

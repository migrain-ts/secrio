// @vitest-environment node
import { expect, test } from 'vitest'
import { assertNoMockInProduction, assertProductionBuild } from '@/api/mock-guard'

test('a vite build refuses test/development modes and a non-production NODE_ENV', () => {
  expect(() => assertProductionBuild('build', 'test', undefined)).toThrow(/"test" vite build/)
  expect(() => assertProductionBuild('build', 'development', undefined)).toThrow(/"development" vite build/)
  expect(() => assertProductionBuild('build', 'production', 'test')).toThrow(/NODE_ENV is "test"/)
  expect(() => assertProductionBuild('build', 'production', 'development')).toThrow(/NODE_ENV/)
})

test('a production build, and serve/vitest in any mode, are fine', () => {
  expect(() => assertProductionBuild('build', 'production', undefined)).not.toThrow()
  expect(() => assertProductionBuild('build', 'production', '')).not.toThrow()
  expect(() => assertProductionBuild('build', 'production', 'production')).not.toThrow()
  // vitest loads the config with command 'serve', mode 'test', NODE_ENV 'test'
  expect(() => assertProductionBuild('serve', 'test', 'test')).not.toThrow()
  expect(() => assertProductionBuild('serve', 'development', 'development')).not.toThrow()
})

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

// @vitest-environment node
// `wails build` runs the frontend build itself (wails.json frontend:build), so
// the dist checks (no mock, nothing remote) must be part of that command.
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { expect, test } from 'vitest'

const read = (rel: string) => JSON.parse(readFileSync(fileURLToPath(new URL(rel, import.meta.url)), 'utf8'))

test('wails builds the frontend with build:release', () => {
  expect(read('../../../cmd/secrio-desktop/wails.json')['frontend:build']).toBe('pnpm run build:release')
})

test('build:release checks the dist right after building it', () => {
  const s = read('../package.json').scripts as Record<string, string>
  expect(s['build:release']).toBe('tsc -b && vite build && DIST_CHECK=1 vitest run src/assets.test.ts')
  expect(s['test:dist']).toBe('pnpm run build:release')
})

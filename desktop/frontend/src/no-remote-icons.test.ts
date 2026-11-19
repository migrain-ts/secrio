// @vitest-environment node
// Runtime Iconify fetches icons from api.iconify.design, which the CSP
// (connect-src 'self') and offline use forbid. Icons are owned components in
// src/components/icons (Iconoir, installed with Better Design install-icons).
import { readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, test } from 'vitest'

const src = fileURLToPath(new URL('.', import.meta.url))

function sources(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((d) => {
    const p = join(dir, d.name)
    if (d.isDirectory()) return sources(p)
    return /\.(ts|tsx)$/.test(d.name) && p !== fileURLToPath(import.meta.url) ? [p] : []
  })
}

test('no source imports @iconify or references api.iconify.design', () => {
  const bad = sources(src).filter((f) => {
    const t = readFileSync(f, 'utf8')
    return /from\s+["']@iconify\//.test(t) || /import\(\s*["']@iconify\//.test(t) || t.includes('api.iconify.design')
  })
  expect(bad.map((f) => f.slice(src.length))).toEqual([])
})

test('@iconify is not a dependency', () => {
  const pkg = JSON.parse(readFileSync(join(src, '../package.json'), 'utf8')) as Record<string, Record<string, string>>
  const deps = Object.keys({ ...pkg.dependencies, ...pkg.devDependencies })
  expect(deps.filter((d) => d.startsWith('@iconify/'))).toEqual([])
})

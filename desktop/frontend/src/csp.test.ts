// @vitest-environment node
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { expect, test } from 'vitest'

const html = readFileSync(fileURLToPath(new URL('../index.html', import.meta.url)), 'utf8')

test('index.html has a strict CSP meta tag', () => {
  const m = html.match(/<meta\s+http-equiv="Content-Security-Policy"\s+content="([^"]+)"/)
  expect(m).not.toBeNull()
  const csp = m![1]
  expect(csp).toContain("default-src 'self'")
  expect(csp).toContain("script-src 'self'")
  expect(csp).toContain("object-src 'none'")
  expect(csp).not.toContain('unsafe-eval')
})

test('index.html loads nothing remote', () => {
  expect(html).not.toMatch(/https?:\/\//)
})

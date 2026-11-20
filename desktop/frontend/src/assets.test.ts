// @vitest-environment node
// Checks the production build: nothing remote, fonts bundled. Runs only with
// DIST_CHECK=1, which `pnpm test:dist` sets right after building, so plain
// `pnpm test` never depends on (or trusts) a stale dist.
import { existsSync, readdirSync, readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'

const dist = fileURLToPath(new URL('../dist/', import.meta.url))
const enabled = process.env.DIST_CHECK === '1'

// URL strings allowed in the bundle. None is ever fetched:
// - XML namespaces used by React DOM for SVG/MathML;
// - React's minified error text ("visit https://react.dev/errors/N");
// - Tailwind's license banner comment;
// - Base UI's minified error text ("visit https://base-ui.com/production-error?code=N").
const ALLOWED = [
  'http://www.w3.org/',
  'https://react.dev/errors/',
  'https://tailwindcss.com',
  'https://base-ui.com/production-error',
]

describe.runIf(enabled)('dist', () => {
  test('dist exists (run pnpm test:dist)', () => {
    expect(existsSync(dist)).toBe(true)
  })

  const assets = () => readdirSync(`${dist}assets`)
  const text = (f: string) => readFileSync(`${dist}assets/${f}`, 'utf8')

  test('CSS and JS reference no remote URL', () => {
    const files = assets().filter((f) => f.endsWith('.css') || f.endsWith('.js'))
    expect(files.length).toBeGreaterThan(0)
    const found: string[] = []
    for (const f of files) {
      for (const m of text(f).matchAll(/https?:\/\/[^\s"'`)\\]*/g)) {
        if (!ALLOWED.some((a) => m[0].startsWith(a))) found.push(`${f}: ${m[0]}`)
      }
    }
    expect(found).toEqual([])
  })

  test('CSS has no @import and no remote url()', () => {
    for (const f of assets().filter((f) => f.endsWith('.css'))) {
      expect(text(f)).not.toMatch(/@import/)
      expect(text(f)).not.toMatch(/url\(\s*["']?(https?:)?\/\//)
    }
  })

  test('the Inter font files are bundled and referenced locally', () => {
    const fonts = assets().filter((f) => /^inter-.*\.woff2$/.test(f))
    expect(fonts.some((f) => f.startsWith('inter-latin-wght-normal'))).toBe(true)
    const css = assets()
      .filter((f) => f.endsWith('.css'))
      .map(text)
      .join('\n')
    expect(css).toMatch(/@font-face\{font-family:["']?Inter Variable/)
    for (const f of fonts) expect(css).toContain(`/assets/${f}`)
  })

  test('the mock api is not in the production bundle', () => {
    const js = assets()
      .filter((f) => f.endsWith('.js'))
      .map(text)
      .join('\n')
    // MOCK_MARKER in src/api/mock.ts, and distinctive seed data/password
    for (const s of ['__SECRIO_MOCK_BUNDLE__', 'fake-jwt-signing-secret-0000', 'hotenb', 'correct horse']) {
      expect([s, js.includes(s)]).toEqual([s, false])
    }
  })

  test('nothing in dist refers to api.iconify.design', () => {
    const all = readdirSync(`${dist}assets`)
      .filter((f) => /\.(js|css)$/.test(f))
      .map(text)
      .join('\n')
    expect(all + readFileSync(`${dist}index.html`, 'utf8')).not.toContain('api.iconify.design')
  })

  test('index.html is dark, keeps the CSP and loads nothing remote', () => {
    const html = readFileSync(`${dist}index.html`, 'utf8')
    expect(html).toMatch(/<html[^>]*class="dark"/)
    expect(html).toContain('http-equiv="Content-Security-Policy"')
    expect(html).toContain("font-src 'self' data:")
    expect(html).not.toMatch(/https?:\/\//)
  })
})

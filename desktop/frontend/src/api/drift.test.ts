// Drift guards: the generated Wails bindings (from the Go code) are the source
// of truth; the Api interface, the real client and the mock must follow them.
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'
import { createRealApi } from '@/api/client'
import { createMockApi, MOCK_IMPORT_PATH } from '@/api/mock'

const read = (rel: string) => readFileSync(fileURLToPath(new URL(rel, import.meta.url)), 'utf8')
const apiDts = read('../../wailsjs/go/desktop/API.d.ts')
const modelsTs = read('../../wailsjs/go/models.ts')

/** Go method name -> number of arguments, from API.d.ts. */
function boundFunctions(): Map<string, number> {
  const out = new Map<string, number>()
  for (const m of apiDts.matchAll(/^export function (\w+)\(([^)]*)\)/gm)) {
    out.set(m[1], m[2].trim() === '' ? 0 : m[2].split(',').length)
  }
  return out
}

/** Class name -> declared fields, from models.ts. */
function dtoFields(): Map<string, string[]> {
  const out = new Map<string, string[]>()
  for (const m of modelsTs.matchAll(/export class (\w+) \{([\s\S]*?)static createFrom/g)) {
    out.set(m[1], [...m[2].matchAll(/^\s*(\w+): /gm)].map((f) => f[1]).sort())
  }
  return out
}

const camel = (s: string) => s[0].toLowerCase() + s.slice(1)
const apiKeys = (o: object) => Object.keys(o).filter((k) => k !== '__test')

describe('Api surface vs API.d.ts', () => {
  const bound = boundFunctions()

  test('API.d.ts declares the 20 bound methods', () => {
    expect(bound.size).toBe(20)
  })

  test('real client and mock implement exactly those methods', () => {
    const want = [...bound.keys()].map(camel).sort()
    expect(apiKeys(createRealApi()).sort()).toEqual(want)
    expect(apiKeys(createMockApi()).sort()).toEqual(want)
  })

  test('each method takes the same number of arguments', () => {
    const real = createRealApi() as unknown as Record<string, (...a: unknown[]) => unknown>
    const mock = createMockApi() as unknown as Record<string, (...a: unknown[]) => unknown>
    for (const [name, n] of bound) {
      expect([name, real[camel(name)].length]).toEqual([name, n])
      expect([name, mock[camel(name)].length]).toEqual([name, n])
    }
  })
})

describe('mock responses vs models.ts DTO fields', () => {
  test('every DTO class matches the keys of a mock response', async () => {
    const m = createMockApi()
    const project = (await m.listProjects()).find((p) => p.name === 'hotenb')!
    const env = (await m.listEnvironments(project.id)).find((e) => e.name === 'Staging')!
    const path = await m.pickEnvFile()
    const samples: Record<string, object[]> = {
      VaultDTO: [(await m.listVaults())[0]],
      CurrentVaultDTO: [await m.currentVault()],
      ProjectDTO: [project, await m.createProject('new-project')],
      EnvDTO: [env, await m.createEnvironment(project.id, 'QA')],
      SecretDTO: [(await m.listSecrets(env.id))[0]],
      ImportPreview: [await m.previewImport(env.id, path)],
      CopyResult: [await m.copySecret(env.id, 'SENTRY_DSN')],
    }
    samples.VaultDTO.push(await m.createVault('other', 'pw')) // last: it switches the current vault
    const fields = dtoFields()
    expect([...fields.keys()].sort()).toEqual(Object.keys(samples).sort())
    for (const [cls, objs] of Object.entries(samples)) {
      for (const o of objs) expect([cls, Object.keys(o).sort()]).toEqual([cls, fields.get(cls)])
    }
    // the nested vault of CurrentVaultDTO is a VaultDTO
    expect(Object.keys((await m.currentVault()).vault).sort()).toEqual(fields.get('VaultDTO'))
    expect(path).toBe(MOCK_IMPORT_PATH)
  })

  test('no generated DTO has a value field', () => {
    for (const [cls, f] of dtoFields()) expect([cls, f.includes('value')]).toEqual([cls, false])
  })
})

describe('background colour', () => {
  test('index.css token, index.html and main.go agree', () => {
    expect(read('../index.css')).toMatch(/--background: oklch\(0\.139 0\.003 246\.3\);/)
    expect(read('../../index.html')).toMatch(/<html[^>]*class="dark"[^>]*background-color: rgb\(8 9 10\)/)
    expect(read('../../../../cmd/secrio-desktop/main.go')).toContain('BackgroundColour: options.NewRGBA(8, 9, 10, 255)')
  })
})

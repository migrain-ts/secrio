import { describe, expect, test } from 'vitest'
import type { ApiError } from '@/api/errors'
import { createMockApi, MOCK_IMPORT_PATH, MOCK_MARKER, type MockApi } from '@/api/mock'
import type { ErrorCode } from '@/api/types'

async function code(p: Promise<unknown>): Promise<ErrorCode | 'resolved'> {
  try {
    await p
    return 'resolved'
  } catch (e) {
    return (e as ApiError).code
  }
}

async function envId(m: MockApi, project: string, env: string): Promise<string> {
  const p = (await m.listProjects()).find((x) => x.name === project)!
  return (await m.listEnvironments(p.id)).find((x) => x.name === env)!.id
}

describe('seed data', () => {
  test('vault work, current and unlocked by default', async () => {
    const m = createMockApi()
    expect(await m.listVaults()).toEqual([{ id: 'v-work', name: 'work' }])
    expect(await m.currentVault()).toEqual({ vault: { id: 'v-work', name: 'work' }, unlocked: true })
  })

  test('projects, envs and counts, sorted by name', async () => {
    const m = createMockApi()
    const ps = await m.listProjects()
    expect(ps.map((p) => p.name)).toEqual(['hotenb', 'infra', 'portfolio'])
    const hotenb = ps[0]
    expect(hotenb.envCount).toBe(3)
    const envs = await m.listEnvironments(hotenb.id)
    expect(envs.map((e) => e.name)).toEqual(['Development', 'Production', 'Staging'])
    expect(hotenb.secretCount).toBe(envs.reduce((n, e) => n + e.secretCount, 0))
    const portfolio = ps.find((p) => p.name === 'portfolio')!
    expect((await m.listEnvironments(portfolio.id)).map((e) => e.name)).toEqual(['Development', 'Production'])
    const infra = ps.find((p) => p.name === 'infra')!
    expect((await m.listEnvironments(infra.id)).map((e) => e.name)).toEqual(['Production'])
  })

  test('staging misses the five core secrets that development has', async () => {
    const m = createMockApi()
    const dev = (await m.listSecrets(await envId(m, 'hotenb', 'Development'))).map((s) => s.name)
    const stg = (await m.listSecrets(await envId(m, 'hotenb', 'Staging'))).map((s) => s.name)
    for (const n of ['DATABASE_URL', 'JWT_SECRET', 'API_KEY', 'STRIPE_KEY', 'REDIS_URL']) {
      expect(dev).toContain(n)
      expect(stg).not.toContain(n)
    }
  })

  test('secrets are sorted by name, case-sensitively', async () => {
    const m = createMockApi()
    const e = await envId(m, 'hotenb', 'Development')
    await m.saveSecret(e, 'a_lower', 'v', 'text')
    const names = (await m.listSecrets(e)).map((s) => s.name)
    expect(names).toEqual([...names].sort((a, b) => (a < b ? -1 : a > b ? 1 : 0)))
    expect(names.at(-1)).toBe('a_lower')
  })

  test('projects sort case-insensitively', async () => {
    const m = createMockApi()
    await m.createProject('Alpha')
    await m.createProject('zeta')
    expect((await m.listProjects()).map((p) => p.name)).toEqual(['Alpha', 'hotenb', 'infra', 'portfolio', 'zeta'])
  })
})

describe('values never leak into lists', () => {
  test('no list response has a value key', async () => {
    const m = createMockApi()
    const all: unknown[] = [await m.listVaults(), await m.listProjects()]
    for (const p of await m.listProjects()) {
      const envs = await m.listEnvironments(p.id)
      all.push(envs)
      for (const e of envs) all.push(await m.listSecrets(e.id))
    }
    const text = JSON.stringify(all)
    expect(text).not.toMatch(/"value"/)
    expect(text).not.toContain('postgres://')
    for (const list of all as Record<string, unknown>[][]) {
      for (const item of list) expect(Object.keys(item)).not.toContain('value')
    }
  })

  test('reveal is the only way to the value and round-trips a save', async () => {
    const m = createMockApi()
    const e = await envId(m, 'hotenb', 'Staging')
    await m.saveSecret(e, 'NEW_ONE', 'hello world', 'token')
    expect(await m.revealSecret(e, 'NEW_ONE')).toBe('hello world')
    const s = (await m.listSecrets(e)).find((x) => x.name === 'NEW_ONE')!
    expect(s).toMatchObject({ name: 'NEW_ONE', type: 'token', description: '' })
    expect(s.createdAt).toMatch(/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ$/)
    expect(JSON.stringify(s)).not.toContain('hello world')
  })

  test('the seeded value is reachable through revealSecret', async () => {
    const m = createMockApi()
    const e = await envId(m, 'hotenb', 'Development')
    expect(await m.revealSecret(e, 'DATABASE_URL')).toMatch(/^postgres:\/\//)
  })
})

describe('saveSecret', () => {
  test('update keeps description and createdAt; empty type keeps the type', async () => {
    const m = createMockApi()
    const e = await envId(m, 'hotenb', 'Development')
    const before = (await m.listSecrets(e)).find((s) => s.name === 'DATABASE_URL')!
    expect(before.description).not.toBe('')
    await m.saveSecret(e, 'DATABASE_URL', 'postgres://new', '')
    const after = (await m.listSecrets(e)).find((s) => s.name === 'DATABASE_URL')!
    expect(after.description).toBe(before.description)
    expect(after.type).toBe(before.type)
    expect(after.createdAt).toBe(before.createdAt)
    expect(await m.revealSecret(e, 'DATABASE_URL')).toBe('postgres://new')
    await m.saveSecret(e, 'DATABASE_URL', 'postgres://new2', 'text')
    expect((await m.listSecrets(e)).find((s) => s.name === 'DATABASE_URL')!.type).toBe('text')
  })

  test('new secret with empty type becomes text', async () => {
    const m = createMockApi()
    const e = await envId(m, 'infra', 'Production')
    await m.saveSecret(e, 'X', 'v', '')
    expect((await m.listSecrets(e)).find((s) => s.name === 'X')!.type).toBe('text')
  })

  test('name, value and type validation', async () => {
    const m = createMockApi()
    const e = await envId(m, 'hotenb', 'Development')
    for (const bad of ['', '1ABC', 'A-B', 'A B', 'é', 'A.B']) {
      expect(await code(m.saveSecret(e, bad, 'v', 'text'))).toBe('invalid_name')
    }
    expect(await code(m.saveSecret(e, '_ok9', 'v', 'text'))).toBe('resolved')
    expect(await code(m.saveSecret(e, 'A', '', 'text'))).toBe('invalid_value')
    expect(await code(m.saveSecret(e, 'A', 'a\0b', 'text'))).toBe('invalid_value')
    expect(await code(m.saveSecret(e, 'A', 'v', 'banana' as never))).toBe('invalid_value')
    expect(await code(m.saveSecret('e-nope', 'A', 'v', 'text'))).toBe('not_found')
  })

  test('1 MiB boundary counts UTF-8 bytes', async () => {
    const m = createMockApi()
    const e = await envId(m, 'hotenb', 'Development')
    const mib = 1 << 20
    expect(await code(m.saveSecret(e, 'BIG', 'x'.repeat(mib), 'text'))).toBe('resolved')
    expect(await code(m.saveSecret(e, 'BIG', 'x'.repeat(mib + 1), 'text'))).toBe('invalid_value')
    // 'é' is two bytes: mib/2 of them is exactly 1 MiB, one more is over.
    expect(await code(m.saveSecret(e, 'BIG', 'é'.repeat(mib / 2), 'text'))).toBe('resolved')
    expect(await code(m.saveSecret(e, 'BIG', 'é'.repeat(mib / 2) + 'x', 'text'))).toBe('invalid_value')
  })

  test('validation happens before the lock check, like the facade', async () => {
    const m = createMockApi({ locked: true })
    expect(await code(m.saveSecret('e', 'bad name', 'v', 'text'))).toBe('invalid_name')
    expect(await code(m.saveSecret('e', 'OK', 'v', 'text'))).toBe('locked')
  })
})

describe('structure errors', () => {
  test('duplicate and invalid names', async () => {
    const m = createMockApi()
    expect(await code(m.createProject('hotenb'))).toBe('duplicate')
    expect(await code(m.createProject('Hotenb'))).toBe('resolved') // exact match only, like the Core
    expect(await code(m.createProject('   '))).toBe('invalid_name')
    expect(await code(m.createProject('a\nb'))).toBe('invalid_name')
    expect(await code(m.createProject('a\u001bb'))).toBe('invalid_name')
    const hotenb = (await m.listProjects()).find((p) => p.name === 'hotenb')!
    expect(await code(m.createEnvironment(hotenb.id, 'Staging'))).toBe('duplicate')
    expect(await code(m.createEnvironment(hotenb.id, ''))).toBe('invalid_name')
    expect(await code(m.createEnvironment('p-nope', 'QA'))).toBe('not_found')
    const qa = await m.createEnvironment(hotenb.id, 'QA')
    expect(qa).toMatchObject({ name: 'QA', secretCount: 0 })
  })

  test('not_found for unknown ids and names', async () => {
    const m = createMockApi()
    const e = await envId(m, 'hotenb', 'Development')
    expect(await code(m.deleteProject('p-nope'))).toBe('not_found')
    expect(await code(m.deleteEnvironment('e-nope'))).toBe('not_found')
    expect(await code(m.listEnvironments('p-nope'))).toBe('not_found') // like envDTOs in Go
    expect(await code(m.listSecrets('e-nope'))).toBe('not_found')
    expect(await code(m.deleteSecret(e, 'NOPE'))).toBe('not_found')
    expect(await code(m.revealSecret(e, 'NOPE'))).toBe('not_found')
    expect(await code(m.copySecret(e, 'NOPE'))).toBe('not_found')
    expect(await code(m.selectVault('v-nope'))).toBe('not_found')
    expect(await code(m.unlock('v-nope', 'correct horse'))).toBe('not_found')
  })

  test('deletes cascade', async () => {
    const m = createMockApi()
    const hotenb = (await m.listProjects()).find((p) => p.name === 'hotenb')!
    const stg = await envId(m, 'hotenb', 'Staging')
    await m.deleteEnvironment(stg)
    expect((await m.listProjects()).find((p) => p.name === 'hotenb')!.envCount).toBe(2)
    expect(await code(m.listSecrets(stg))).toBe('not_found')
    await m.deleteProject(hotenb.id)
    expect((await m.listProjects()).map((p) => p.name)).toEqual(['infra', 'portfolio'])
  })

  test('deleteSecret removes it', async () => {
    const m = createMockApi()
    const e = await envId(m, 'hotenb', 'Development')
    await m.deleteSecret(e, 'API_KEY')
    expect((await m.listSecrets(e)).map((s) => s.name)).not.toContain('API_KEY')
    expect(await code(m.deleteSecret(e, 'bad name'))).toBe('invalid_name')
  })
})

describe('session', () => {
  test('lock makes every data call fail with locked; unlock restores', async () => {
    const m = createMockApi()
    const e = await envId(m, 'hotenb', 'Development')
    await m.lock()
    expect(await m.currentVault()).toEqual({ vault: { id: 'v-work', name: 'work' }, unlocked: false })
    const calls: (() => Promise<unknown>)[] = [
      () => m.listProjects(),
      () => m.createProject('x'),
      () => m.deleteProject('p-hotenb'),
      () => m.listEnvironments('p-hotenb'),
      () => m.createEnvironment('p-hotenb', 'x'),
      () => m.deleteEnvironment(e),
      () => m.listSecrets(e),
      () => m.saveSecret(e, 'A', 'v', 'text'),
      () => m.deleteSecret(e, 'API_KEY'),
      () => m.revealSecret(e, 'API_KEY'),
      () => m.copySecret(e, 'API_KEY'),
      () => m.previewImport(e, MOCK_IMPORT_PATH),
    ]
    await m.pickEnvFile() // the picker needs no session; preview then hits the lock
    for (const c of calls) expect(await code(c())).toBe('locked')
    expect(await m.listVaults()).toHaveLength(1) // no session needed
    expect(await code(m.unlock('v-work', 'wrong'))).toBe('invalid_password')
    await m.unlock('v-work', 'correct horse')
    expect(await m.listProjects()).toHaveLength(3)
  })

  test('opts.locked and a custom password', async () => {
    const m = createMockApi({ locked: true, password: 'pw' })
    expect((await m.currentVault()).unlocked).toBe(false)
    expect(await code(m.listProjects())).toBe('locked')
    expect(await code(m.unlock('v-work', 'correct horse'))).toBe('invalid_password')
    await m.unlock('v-work', 'pw')
    expect((await m.currentVault()).unlocked).toBe(true)
  })

  test('session expiry and CLI lock hooks', async () => {
    const m = createMockApi()
    m.__test.expireSession()
    expect(await code(m.listProjects())).toBe('locked')
    await m.unlock('v-work', 'correct horse')
    m.__test.lockedByCli()
    expect(await code(m.listProjects())).toBe('locked')
  })

  test('no vault: first run', async () => {
    const m = createMockApi({ noVault: true })
    expect(await m.listVaults()).toEqual([])
    expect(await m.currentVault()).toEqual({ vault: { id: '', name: '' }, unlocked: false })
    expect(await code(m.listProjects())).toBe('no_vault')
    expect(await code(m.lock())).toBe('no_vault')
    const v = await m.createVault('personal', 'pw')
    expect(v.name).toBe('personal')
    expect(await m.currentVault()).toEqual({ vault: v, unlocked: true })
    expect(await m.listProjects()).toEqual([])
  })

  test('createVault validation', async () => {
    const m = createMockApi()
    expect(await code(m.createVault('WORK', 'pw'))).toBe('duplicate')
    for (const bad of ['', '.hidden', '-x', 'a..b', 'con', 'a b', 'x'.repeat(65)]) {
      expect(await code(m.createVault(bad, 'pw'))).toBe('invalid_name')
    }
    expect(await code(m.createVault('ok', ''))).toBe('invalid_value')
  })

  test('two vaults: selection and current', async () => {
    const m = createMockApi()
    const v = await m.createVault('personal', 'pw2')
    expect((await m.listVaults()).map((x) => x.name)).toEqual(['personal', 'work'])
    await m.selectVault('v-work')
    expect((await m.currentVault()).vault.name).toBe('work')
    await m.selectVault(v.id)
    expect(await m.listProjects()).toEqual([])
  })
})

describe('clipboard', () => {
  test('readable clipboard: cleared in 30 s, value lands in the mock clipboard', async () => {
    const m = createMockApi()
    const e = await envId(m, 'hotenb', 'Development')
    expect(await m.copySecret(e, 'JWT_SECRET')).toEqual({ clearInSeconds: 30 })
    expect(m.__test.clipboardText()).toBe(await m.revealSecret(e, 'JWT_SECRET'))
    await m.lock()
    expect(m.__test.clipboardText()).toBe('') // lock clears our copy
  })

  test('unreadable clipboard: no auto clear', async () => {
    const m = createMockApi({ clipboardReadable: false })
    const e = await envId(m, 'hotenb', 'Development')
    expect(await m.copySecret(e, 'JWT_SECRET')).toEqual({ clearInSeconds: 0 })
  })
})

describe('import', () => {
  test('pick, preview, apply with skip', async () => {
    const m = createMockApi()
    const e = await envId(m, 'hotenb', 'Staging')
    const path = await m.pickEnvFile()
    expect(path).toBe(MOCK_IMPORT_PATH)
    const before = await m.revealSecret(e, 'SENTRY_DSN')
    const pv = await m.previewImport(e, path)
    expect(pv.duplicate).toEqual([...pv.duplicate].sort())
    expect(pv.new).toEqual([...pv.new].sort())
    expect(pv.duplicate).toContain('SENTRY_DSN')
    expect(pv.new).toContain('DATABASE_URL')
    await m.applyImport(e, path, 'skip')
    const names = (await m.listSecrets(e)).map((s) => s.name)
    for (const n of pv.new) expect(names).toContain(n)
    expect(await m.revealSecret(e, 'SENTRY_DSN')).toBe(before)
    expect((await m.listSecrets(e)).find((s) => s.name === 'DATABASE_URL')!.type).toBe('text')
    // the preview is consumed by a successful apply
    expect(await code(m.applyImport(e, path, 'skip'))).toBe('conflict')
  })

  test('overwrite changes the value only', async () => {
    const m = createMockApi()
    const e = await envId(m, 'hotenb', 'Staging')
    m.__test.setFileContent(MOCK_IMPORT_PATH, { SENTRY_DSN: 'https://new@sentry.example/1' })
    const path = await m.pickEnvFile()
    const old = (await m.listSecrets(e)).find((s) => s.name === 'SENTRY_DSN')!
    expect(await m.previewImport(e, path)).toEqual({ new: [], duplicate: ['SENTRY_DSN'] })
    await m.applyImport(e, path, 'overwrite')
    expect(await m.revealSecret(e, 'SENTRY_DSN')).toBe('https://new@sentry.example/1')
    const now = (await m.listSecrets(e)).find((s) => s.name === 'SENTRY_DSN')!
    expect(now.type).toBe(old.type)
    expect(now.description).toBe(old.description)
  })

  test('conflict when the file changed between preview and apply', async () => {
    const m = createMockApi()
    const e = await envId(m, 'hotenb', 'Staging')
    const path = await m.pickEnvFile()
    await m.previewImport(e, path)
    m.__test.setFileContent(path, { ONLY_THIS: 'x' })
    expect(await code(m.applyImport(e, path, 'skip'))).toBe('conflict')
    expect((await m.listSecrets(e)).map((s) => s.name)).not.toContain('ONLY_THIS')
    // preview again then apply works
    expect(await m.previewImport(e, path)).toEqual({ new: ['ONLY_THIS'], duplicate: [] })
    await m.applyImport(e, path, 'skip')
    expect((await m.listSecrets(e)).map((s) => s.name)).toContain('ONLY_THIS')
  })

  test('conflict when the vault changed between preview and apply', async () => {
    const m = createMockApi()
    const e = await envId(m, 'hotenb', 'Staging')
    const path = await m.pickEnvFile()
    const pv = await m.previewImport(e, path)
    await m.saveSecret(e, pv.new[0], 'v', 'text') // a new name became a duplicate
    expect(await code(m.applyImport(e, path, 'skip'))).toBe('conflict')
  })

  test('import errors', async () => {
    const m = createMockApi()
    const e = await envId(m, 'hotenb', 'Staging')
    expect(await code(m.previewImport(e, '/not/picked.env'))).toBe('invalid_value')
    const path = await m.pickEnvFile()
    expect(await code(m.applyImport(e, path, 'skip'))).toBe('conflict') // no preview yet
    expect(await code(m.applyImport(e, path, 'merge' as never))).toBe('invalid_value')
    m.__test.setFileContent(path, null)
    expect(await code(m.previewImport(e, path))).toBe('not_found')
    m.__test.setFileContent(path, { 'bad name': 'x' })
    expect(await code(m.previewImport(e, path))).toBe('invalid_value')
  })

  test('cancelled picker returns an empty path', async () => {
    const m = createMockApi({ pickResult: '' })
    expect(await m.pickEnvFile()).toBe('')
  })

  test('setPickResult: the next picks return another path, which becomes allowed', async () => {
    const m = createMockApi()
    const e = await envId(m, 'hotenb', 'Staging')
    m.__test.setFileContent('/other/.env', { OTHER_NAME: 'x' })
    expect(await code(m.previewImport(e, '/other/.env'))).toBe('invalid_value') // not picked yet
    m.__test.setPickResult('/other/.env')
    expect(await m.pickEnvFile()).toBe('/other/.env')
    expect(await m.previewImport(e, '/other/.env')).toEqual({ new: ['OTHER_NAME'], duplicate: [] })
    m.__test.setPickResult('')
    expect(await m.pickEnvFile()).toBe('')
  })
})

describe('bundle marker', () => {
  test('the mock carries MOCK_MARKER, hidden from Object.keys', () => {
    const m = createMockApi()
    expect(MOCK_MARKER).toBe('__SECRIO_MOCK_BUNDLE__')
    expect((m as unknown as Record<string, unknown>)[MOCK_MARKER]).toBe(true)
    expect(Object.keys(m)).not.toContain(MOCK_MARKER)
  })
})

describe('latency', () => {
  test('latencyMs delays responses', async () => {
    const m = createMockApi({ latencyMs: 30 })
    const t = performance.now()
    await m.listVaults()
    expect(performance.now() - t).toBeGreaterThanOrEqual(25)
  })

  test('responses are copies: mutating them does not change the store', async () => {
    const m = createMockApi()
    const ps = await m.listProjects()
    ps[0].name = 'mutated'
    expect((await m.listProjects())[0].name).toBe('hotenb')
  })
})

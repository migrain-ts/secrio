import { describe, expect, test } from 'vitest'
import { type AppEvent, type AppState, initialState, reducer, type Screen } from '@/state/machine'

const at = (screen: Screen, returnTo: AppState['returnTo'] = null): AppState => ({ screen, returnTo })
const project: Screen = { kind: 'project', projectId: 'p1', envId: 'e1' }

describe('reducer', () => {
  const cases: [name: string, from: AppState, ev: AppEvent, to: AppState][] = [
    ['boot -> projects', initialState, { type: 'BOOTED', screen: { kind: 'projects' } }, at({ kind: 'projects' })],
    [
      'boot -> unlock with vault',
      initialState,
      { type: 'BOOTED', screen: { kind: 'unlock', vaultId: 'v1' } },
      at({ kind: 'unlock', vaultId: 'v1' }),
    ],
    ['boot -> create-vault', initialState, { type: 'BOOTED', screen: { kind: 'create-vault' } }, at({ kind: 'create-vault' })],
    [
      'BOOTED after boot is ignored',
      at({ kind: 'projects' }),
      { type: 'BOOTED', screen: { kind: 'create-vault' } },
      at({ kind: 'projects' }),
    ],
    [
      'LOCKED on a project keeps it to return to',
      at(project),
      { type: 'LOCKED', vaultId: 'v1', returnTo: project, notice: 'locked' },
      at({ kind: 'unlock', vaultId: 'v1', notice: 'locked' }, { vaultId: 'v1', screen: project }),
    ],
    [
      'LOCKED without notice (Lock button)',
      at({ kind: 'projects' }),
      { type: 'LOCKED', vaultId: 'v1', returnTo: { kind: 'projects' } },
      at({ kind: 'unlock', vaultId: 'v1' }, { vaultId: 'v1', screen: { kind: 'projects' } }),
    ],
    [
      'LOCKED on the unlock screen is ignored',
      at({ kind: 'unlock', vaultId: 'v1' }),
      { type: 'LOCKED', vaultId: 'v1', returnTo: { kind: 'unlock', vaultId: 'v1' }, notice: 'locked' },
      at({ kind: 'unlock', vaultId: 'v1' }),
    ],
    [
      'LOCKED on create-vault is ignored',
      at({ kind: 'create-vault' }),
      { type: 'LOCKED', vaultId: null, returnTo: { kind: 'create-vault' } },
      at({ kind: 'create-vault' }),
    ],
    [
      'UNLOCKED the same vault restores the selection',
      at({ kind: 'unlock', vaultId: 'v1' }, { vaultId: 'v1', screen: project }),
      { type: 'UNLOCKED', vaultId: 'v1' },
      at(project),
    ],
    [
      'UNLOCKED another vault goes to projects',
      at({ kind: 'unlock', vaultId: 'v1' }, { vaultId: 'v1', screen: project }),
      { type: 'UNLOCKED', vaultId: 'v2' },
      at({ kind: 'projects' }),
    ],
    ['UNLOCKED with nothing to restore', at({ kind: 'unlock', vaultId: null }), { type: 'UNLOCKED', vaultId: 'v1' }, at({ kind: 'projects' })],
    [
      'VAULT_CREATED -> projects, drops returnTo',
      at({ kind: 'create-vault' }, { vaultId: 'v1', screen: project }),
      { type: 'VAULT_CREATED' },
      at({ kind: 'projects' }),
    ],
    [
      'OPEN_PROJECT',
      at({ kind: 'projects' }),
      { type: 'OPEN_PROJECT', projectId: 'p1' },
      at({ kind: 'project', projectId: 'p1', envId: null }),
    ],
    ['OPEN_PROJECTS', at(project), { type: 'OPEN_PROJECTS' }, at({ kind: 'projects' })],
    ['GOTO_CREATE_VAULT', at({ kind: 'unlock', vaultId: null }), { type: 'GOTO_CREATE_VAULT' }, at({ kind: 'create-vault' })],
    ['GOTO_UNLOCK', at({ kind: 'create-vault' }), { type: 'GOTO_UNLOCK', vaultId: 'v2' }, at({ kind: 'unlock', vaultId: 'v2' })],
    ['FATAL from anywhere', at(project, { vaultId: 'v1', screen: project }), { type: 'FATAL' }, at({ kind: 'fatal' })],
    ['fatal is terminal', at({ kind: 'fatal' }), { type: 'OPEN_PROJECTS' }, at({ kind: 'fatal' })],
  ]

  test.each(cases)('%s', (_name, from, ev, to) => {
    expect(reducer(from, ev)).toEqual(to)
  })

  test('is pure: the input state is not mutated', () => {
    const from = at(project)
    const copy = structuredClone(from)
    reducer(from, { type: 'LOCKED', vaultId: 'v1', returnTo: project })
    expect(from).toEqual(copy)
  })
})

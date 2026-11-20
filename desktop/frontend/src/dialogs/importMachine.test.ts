import { describe, expect, test } from 'vitest'
import { ApiError } from '@/api/errors'
import type { ImportPreview, ImportStrategy } from '@/api/types'
import {
  countsLine,
  errorKind,
  type ImportEvent,
  importCount,
  importLabel,
  importMachine,
  type ImportState,
  INITIAL,
  summary,
} from '@/dialogs/importMachine'

const PV = { new: ['A', 'B'], duplicate: ['C'] }
const run = (events: ImportEvent[], from: ImportState = INITIAL) => events.reduce(importMachine, from)
const atPreview = run([{ type: 'PICK' }, { type: 'PICKED', path: '/x/.env' }, { type: 'PREVIEWED', preview: PV }])

describe('importMachine: transitions', () => {
  test.each<[string, ImportEvent[], Partial<ImportState>]>([
    ['pick starts picking', [{ type: 'PICK' }], { step: 'picking', error: null }],
    ['a cancelled picker goes back to idle, no error', [{ type: 'PICK' }, { type: 'PICK_CANCELLED' }], { step: 'idle', error: null, path: null }],
    ['a path starts the preview', [{ type: 'PICK' }, { type: 'PICKED', path: '/x/.env' }], { step: 'previewing', path: '/x/.env' }],
    ['the preview arrives', [{ type: 'PICK' }, { type: 'PICKED', path: '/p' }, { type: 'PREVIEWED', preview: PV }], { step: 'preview', preview: PV, strategy: 'skip' }],
    ['a picker failure is an error without a path', [{ type: 'PICK' }, { type: 'FAILED', error: 'generic' }], { step: 'error', error: 'generic', path: null }],
    ['a preview failure keeps the path', [{ type: 'PICK' }, { type: 'PICKED', path: '/p' }, { type: 'FAILED', error: 'missing' }], { step: 'error', error: 'missing', path: '/p', preview: null }],
  ])('%s', (_, events, want) => {
    expect(run(events)).toMatchObject(want)
  })

  test('strategy, apply, done', () => {
    const s = run([{ type: 'STRATEGY', strategy: 'overwrite' }], atPreview)
    expect(s.strategy).toBe('overwrite')
    const applying = importMachine(s, { type: 'APPLY' })
    expect(applying).toMatchObject({ step: 'applying', strategy: 'overwrite', error: null })
    expect(importMachine(applying, { type: 'APPLIED' }).step).toBe('done')
  })

  test('strategy changes are ignored outside the preview', () => {
    const applying = importMachine(atPreview, { type: 'APPLY' })
    expect(importMachine(applying, { type: 'STRATEGY', strategy: 'overwrite' }).strategy).toBe('skip')
  })

  test('a conflict on apply re-previews the same path, keeps the strategy and the message', () => {
    const s = run([{ type: 'STRATEGY', strategy: 'overwrite' }, { type: 'APPLY' }, { type: 'FAILED', error: 'conflict' }], atPreview)
    expect(s).toMatchObject({ step: 'previewing', path: '/x/.env', strategy: 'overwrite', error: 'conflict', preview: null })
    const fresh = { new: ['A'], duplicate: ['B', 'C'] }
    expect(importMachine(s, { type: 'PREVIEWED', preview: fresh })).toMatchObject({
      step: 'preview',
      preview: fresh,
      error: 'conflict',
      strategy: 'overwrite',
    })
  })

  test('another apply error keeps the strategy and the path', () => {
    const s = run([{ type: 'STRATEGY', strategy: 'overwrite' }, { type: 'APPLY' }, { type: 'FAILED', error: 'invalid' }], atPreview)
    expect(s).toMatchObject({ step: 'error', error: 'invalid', path: '/x/.env', strategy: 'overwrite' })
  })

  test('preview again from an error keeps the strategy', () => {
    const err = run([{ type: 'STRATEGY', strategy: 'overwrite' }, { type: 'APPLY' }, { type: 'FAILED', error: 'generic' }], atPreview)
    expect(importMachine(err, { type: 'REPREVIEW' })).toMatchObject({ step: 'previewing', path: '/x/.env', strategy: 'overwrite', error: null })
  })

  test('choosing another file forgets the old file and resets the strategy', () => {
    const s = run([{ type: 'STRATEGY', strategy: 'overwrite' }, { type: 'PICK' }], atPreview)
    expect(s).toEqual({ ...INITIAL, step: 'picking' })
  })

  test('a cancelled picker from a preview goes back to idle', () => {
    expect(run([{ type: 'PICK' }, { type: 'PICK_CANCELLED' }], atPreview)).toEqual(INITIAL)
  })

  test('reset', () => {
    expect(importMachine(atPreview, { type: 'RESET' })).toEqual(INITIAL)
  })
})

describe('importMachine: copy and counts', () => {
  test.each([
    [{ new: ['A', 'B'], duplicate: ['C'] }, '2 new, 1 already exists'],
    [{ new: ['A'], duplicate: ['C', 'D'] }, '1 new, 2 already exist'],
    [{ new: ['A', 'B', 'C'], duplicate: [] }, '3 new'],
    [{ new: [], duplicate: ['C'] }, 'No new names, 1 already exists'],
    [{ new: [], duplicate: [] }, 'This file has no entries.'],
  ])('countsLine %j', (pv, want) => {
    expect(countsLine(pv)).toBe(want)
  })

  test.each<[ImportPreview, ImportStrategy, number, string]>([
    [PV, 'skip', 2, 'Import 2 secrets'],
    [PV, 'overwrite', 3, 'Import 3 secrets'],
    [{ new: ['A'], duplicate: ['B'] }, 'skip', 1, 'Import 1 secret'],
    [{ new: [], duplicate: ['B'] }, 'overwrite', 1, 'Import 1 secret'],
    [{ new: [], duplicate: ['B'] }, 'skip', 0, 'Close'],
    [{ new: [], duplicate: [] }, 'skip', 0, 'Close'],
  ])('importCount/importLabel %j %s', (pv, strategy, n, label) => {
    expect(importCount(pv, strategy)).toBe(n)
    expect(importLabel(pv, strategy)).toBe(label)
  })

  test('summary', () => {
    expect(summary(PV, 'skip')).toEqual({ imported: 2, replaced: 0, skipped: 1 })
    expect(summary(PV, 'overwrite')).toEqual({ imported: 2, replaced: 1, skipped: 0 })
  })

  test.each([
    ['conflict', 'conflict'],
    ['invalid_value', 'invalid'],
    ['not_found', 'missing'],
    ['unsupported', 'generic'],
    ['internal', 'generic'],
    ['duplicate', 'generic'],
  ] as const)('errorKind %s', (code, kind) => {
    expect(errorKind(new ApiError(code, 'server text'))).toBe(kind)
  })

  test('errorKind of a non-api error is generic', () => {
    expect(errorKind(new Error('x'))).toBe('generic')
  })
})

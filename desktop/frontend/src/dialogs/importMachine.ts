// The .env import dialog's steps, as a pure reducer. It holds names only: the
// facade never returns an imported value.
import { ApiError } from '@/api/errors'
import type { ImportPreview, ImportStrategy } from '@/api/types'

/** UI error kinds; each has one fixed message (never the server text). */
export type ImportErrorKind = 'conflict' | 'invalid' | 'missing' | 'generic'

export type ImportStep = 'idle' | 'picking' | 'previewing' | 'preview' | 'applying' | 'done' | 'error'

export interface ImportState {
  step: ImportStep
  /** The picked file (only paths returned by pickEnvFile). */
  path: string | null
  preview: ImportPreview | null
  strategy: ImportStrategy
  error: ImportErrorKind | null
}

export type ImportEvent =
  | { type: 'PICK' }
  | { type: 'PICK_CANCELLED' }
  | { type: 'PICKED'; path: string }
  | { type: 'PREVIEWED'; preview: ImportPreview }
  | { type: 'STRATEGY'; strategy: ImportStrategy }
  | { type: 'APPLY' }
  | { type: 'APPLIED' }
  | { type: 'FAILED'; error: ImportErrorKind }
  | { type: 'REPREVIEW' }
  | { type: 'RESET' }

export const INITIAL: ImportState = { step: 'idle', path: null, preview: null, strategy: 'skip', error: null }

export function importMachine(s: ImportState, e: ImportEvent): ImportState {
  switch (e.type) {
    case 'PICK': // a new file: forget the old one and its choice
      return { ...INITIAL, step: 'picking' }
    case 'PICK_CANCELLED':
      return INITIAL
    case 'PICKED':
      return { ...s, step: 'previewing', path: e.path }
    case 'PREVIEWED': // keeps a conflict message next to the fresh counts
      return { ...s, step: 'preview', preview: e.preview }
    case 'STRATEGY':
      return s.step === 'preview' ? { ...s, strategy: e.strategy } : s
    case 'APPLY':
      return { ...s, step: 'applying', error: null }
    case 'APPLIED':
      return { ...s, step: 'done' }
    case 'FAILED':
      // The file or the vault changed: preview the same file again, back on
      // the safe choice, so an overwrite of the new set is a fresh decision.
      if (e.error === 'conflict' && s.step === 'applying') {
        return { ...s, step: 'previewing', preview: null, strategy: 'skip', error: 'conflict' }
      }
      return { ...s, step: 'error', error: e.error }
    case 'REPREVIEW':
      return { ...s, step: 'previewing', preview: null, error: null }
    case 'RESET':
      return INITIAL
  }
}

export function errorKind(e: unknown): ImportErrorKind {
  if (!(e instanceof ApiError)) return 'generic'
  if (e.code === 'conflict') return 'conflict'
  if (e.code === 'invalid_value') return 'invalid'
  if (e.code === 'not_found') return 'missing'
  return 'generic'
}

export const ERROR_TEXT: Record<ImportErrorKind, string> = {
  conflict: 'The file or the vault changed. Review the new preview.',
  invalid: "This file has an entry Secrio can't import, or it is too large.",
  missing: 'That file could not be found.',
  generic: 'Import failed. Try again.',
}

export function countsLine(pv: ImportPreview): string {
  const n = pv.new.length
  const m = pv.duplicate.length
  if (n === 0 && m === 0) return 'This file has no entries.'
  const added = n === 0 ? 'No new names' : `${n} new`
  if (m === 0) return added
  return `${added}, ${m} ${m === 1 ? 'already exists' : 'already exist'}`
}

/** How many secrets the import writes. */
export function importCount(pv: ImportPreview, strategy: ImportStrategy): number {
  return pv.new.length + (strategy === 'overwrite' ? pv.duplicate.length : 0)
}

/** The primary button: "Import N secrets", or "Close" when nothing would change. */
export function importLabel(pv: ImportPreview, strategy: ImportStrategy): string {
  const n = importCount(pv, strategy)
  if (n === 0) return 'Close'
  return `Import ${n} ${n === 1 ? 'secret' : 'secrets'}`
}

export interface ImportSummary {
  imported: number
  replaced: number
  skipped: number
}

export function summary(pv: ImportPreview, strategy: ImportStrategy): ImportSummary {
  const m = pv.duplicate.length
  return {
    imported: pv.new.length,
    replaced: strategy === 'overwrite' ? m : 0,
    skipped: strategy === 'skip' ? m : 0,
  }
}

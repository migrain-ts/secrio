import { describe, expect, test } from 'vitest'
import { vaultNameError } from '@/lib/vault-name'

describe('vaultNameError mirrors the facade rule (internal/desktop/app.go)', () => {
  test.each([
    ['work'],
    ['my-vault_2.0'],
    ['A'],
    ['a'.repeat(64)],
    ['com0'],
    ['com10'],
    ['console'],
    ['lpt'],
    ['a.con'],
    ['my-aux'],
    ['_x'],
  ])('%s is accepted', (name) => {
    expect(vaultNameError(name)).toBeNull()
  })

  test.each([
    ['', 'Enter a vault name.'],
    ['a'.repeat(65), 'Use 64 characters or fewer.'],
    ['a b', 'Use only letters, digits, dots, dashes and underscores.'],
    ['a/b', 'Use only letters, digits, dots, dashes and underscores.'],
    ['café', 'Use only letters, digits, dots, dashes and underscores.'],
    ['.', 'Start with a letter, digit or underscore.'],
    ['..', 'Start with a letter, digit or underscore.'],
    ['.hidden', 'Start with a letter, digit or underscore.'],
    ['-x', 'Start with a letter, digit or underscore.'],
    ['a..b', "Don't use two dots in a row."],
    ['con', 'This name is reserved by Windows. Choose another.'],
    ['NUL', 'This name is reserved by Windows. Choose another.'],
    ['Prn', 'This name is reserved by Windows. Choose another.'],
    ['aux', 'This name is reserved by Windows. Choose another.'],
    ['com1', 'This name is reserved by Windows. Choose another.'],
    ['COM9', 'This name is reserved by Windows. Choose another.'],
    ['lpt1', 'This name is reserved by Windows. Choose another.'],
    ['LPT9.txt', 'This name is reserved by Windows. Choose another.'],
    ['con.tar.gz', 'This name is reserved by Windows. Choose another.'],
  ])('%j is refused: %s', (name, msg) => {
    expect(vaultNameError(name)).toBe(msg)
  })
})

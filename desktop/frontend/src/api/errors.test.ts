import { describe, expect, test } from 'vitest'
import { ApiError, parseApiError } from '@/api/errors'
import { ERROR_CODES, type ErrorCode } from '@/api/types'

const json = (code: string, message = 'msg') => JSON.stringify({ code, message })

describe('parseApiError', () => {
  test('knows exactly the ten Go error codes', () => {
    expect([...ERROR_CODES].sort()).toEqual(
      [
        'locked',
        'invalid_password',
        'not_found',
        'duplicate',
        'invalid_name',
        'invalid_value',
        'conflict',
        'unsupported',
        'no_vault',
        'internal',
      ].sort(),
    )
  })

  test.each(ERROR_CODES.map((c) => [c]))('valid JSON with code %s', (code: ErrorCode) => {
    const e = parseApiError(new Error(json(code, 'fixed text')))
    expect(e).toBeInstanceOf(ApiError)
    expect(e).toBeInstanceOf(Error)
    expect(e.code).toBe(code)
    expect(e.message).toBe('fixed text')
  })

  test('accepts the JSON as a plain string', () => {
    const e = parseApiError(json('locked', 'vault is locked'))
    expect(e.code).toBe('locked')
    expect(e.message).toBe('vault is locked')
  })

  test('accepts any object with a message', () => {
    const e = parseApiError({ message: json('duplicate', 'already exists') })
    expect(e.code).toBe('duplicate')
    expect(e.message).toBe('already exists')
  })

  test('returns an ApiError unchanged', () => {
    const orig = new ApiError('conflict', 'preview again')
    expect(parseApiError(orig)).toBe(orig)
  })

  test('a known code without a string message keeps the code', () => {
    const e = parseApiError(new Error(JSON.stringify({ code: 'not_found' })))
    expect(e.code).toBe('not_found')
    expect(e.message).toBe('')
  })

  const unknown: [string, unknown][] = [
    ['unknown code', new Error(json('teapot'))],
    ['garbage text', new Error('TypeError: window.go is undefined')],
    ['empty message', new Error('')],
    ['empty string', ''],
    ['non-JSON string', 'secret=hunter2 failed'],
    ['JSON array', new Error('["locked"]')],
    ['JSON number', new Error('42')],
    ['JSON null', new Error('null')],
    ['code not a string', new Error(JSON.stringify({ code: 1, message: 'x' }))],
    ['object without message', { code: 'locked' }],
    ['object with non-string message', { message: 7 }],
    ['null', null],
    ['undefined', undefined],
    ['number', 42],
  ]
  test.each(unknown)('%s -> internal with a fixed message', (_name, input) => {
    const e = parseApiError(input)
    expect(e).toBeInstanceOf(ApiError)
    expect(e.code).toBe('internal')
    expect(e.message).toBe('unexpected error')
  })
})

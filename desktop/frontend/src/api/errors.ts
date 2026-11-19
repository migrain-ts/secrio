import { ERROR_CODES, type ErrorCode } from '@/api/types'

/** An error from the Go facade, identified by its code. The UI shows fixed
 *  copy per code; message is the facade's fixed, value-free text. */
export class ApiError extends Error {
  readonly code: ErrorCode

  constructor(code: ErrorCode, message: string) {
    super(message)
    this.name = 'ApiError'
    this.code = code
  }
}

function isErrorCode(v: unknown): v is ErrorCode {
  return typeof v === 'string' && (ERROR_CODES as readonly string[]).includes(v)
}

/**
 * Turns whatever a binding rejected with into an ApiError. Wails v2 rejects
 * with an Error whose message is exactly the Go err.Error(), i.e. our JSON
 * {"code","message"}; a plain string or any object with a string message is
 * accepted too. Anything else (non-JSON, unknown code, empty) becomes
 * internal/"unexpected error": raw unknown text never reaches the UI.
 */
export function parseApiError(e: unknown): ApiError {
  if (e instanceof ApiError) return e
  let text: unknown
  if (typeof e === 'string') text = e
  else if (typeof e === 'object' && e !== null && 'message' in e) text = e.message
  if (typeof text === 'string' && text !== '') {
    try {
      const v: unknown = JSON.parse(text)
      if (typeof v === 'object' && v !== null && 'code' in v && isErrorCode(v.code)) {
        const msg = 'message' in v && typeof v.message === 'string' ? v.message : ''
        return new ApiError(v.code, msg)
      }
    } catch {
      // not JSON: fall through
    }
  }
  return new ApiError('internal', 'unexpected error')
}

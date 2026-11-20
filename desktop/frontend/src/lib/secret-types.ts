import type { SecretType } from '@/api/types'

/** Human labels of the Core secret types, shown in badges and the type select. */
export const TYPE_LABELS: Record<SecretType, string> = {
  text: 'Text',
  password: 'Password',
  api_key: 'API key',
  token: 'Token',
  url: 'URL',
  private_key: 'Private key',
  certificate: 'Certificate',
  json: 'JSON',
}

/** The masked value: the same run of dots for every secret, whatever its length. */
export const MASK = '••••••••••'

const DATE = new Intl.DateTimeFormat('en-US', { month: 'short', day: 'numeric', year: 'numeric', timeZone: 'UTC' })

/** "Nov 12, 2026" from an RFC3339 time (en dash when unreadable). */
export function shortDate(rfc3339: string): string {
  const d = new Date(rfc3339)
  return Number.isNaN(d.getTime()) ? '–' : DATE.format(d)
}

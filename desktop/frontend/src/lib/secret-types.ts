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

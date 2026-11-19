// Mirrors of the Go DTOs in internal/desktop/dto.go, field for field by json
// tag. No DTO has a value field: a secret value only leaves Go through
// revealSecret. drift.test.ts checks these against the generated
// wailsjs/go/models.ts.

/** Secret types of the Core (internal/domain/entities.go). */
export const SECRET_TYPES = [
  'text',
  'password',
  'api_key',
  'token',
  'url',
  'private_key',
  'certificate',
  'json',
] as const
export type SecretType = (typeof SECRET_TYPES)[number]

/** Error codes of internal/desktop/errors.go. */
export const ERROR_CODES = [
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
] as const
export type ErrorCode = (typeof ERROR_CODES)[number]

export type ImportStrategy = 'skip' | 'overwrite'

export interface VaultDTO {
  id: string
  name: string
}

/** CurrentVaultDTO. vault.id is "" when there is no current vault. */
export interface CurrentVault {
  vault: VaultDTO
  unlocked: boolean
}

/** A project with its totals (counts only). */
export interface ProjectDTO {
  id: string
  name: string
  envCount: number
  secretCount: number
}

export interface EnvDTO {
  id: string
  name: string
  secretCount: number
}

/** Secret metadata. Times are RFC3339 UTC. Never a value. */
export interface SecretDTO {
  name: string
  type: SecretType
  description: string
  createdAt: string
  updatedAt: string
}

/** Names a .env import would add (new) or touch (duplicate). Names only. */
export interface ImportPreview {
  new: string[]
  duplicate: string[]
}

/** Seconds until the copied value is cleared; 0 = not cleared automatically. */
export interface CopyResult {
  clearInSeconds: number
}

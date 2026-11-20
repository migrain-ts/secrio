// Client copy of the vault naming rule of the facade (validVaultName in
// internal/desktop/app.go, itself a copy of the CLI rule). The server stays the
// authority; this only gives a field message before the round trip.

export const VAULT_NAME_HINT = 'Letters, digits, dots, dashes and underscores. Up to 64 characters.'

/** The message for an invalid vault name, or null when it is valid. */
export function vaultNameError(name: string): string | null {
  if (name === '') return 'Enter a vault name.'
  if (name.length > 64) return 'Use 64 characters or fewer.'
  if (!/^[A-Za-z0-9._-]+$/.test(name)) return 'Use only letters, digits, dots, dashes and underscores.'
  if (name.startsWith('.') || name.startsWith('-')) return 'Start with a letter, digit or underscore.'
  if (name.includes('..')) return "Don't use two dots in a row."
  if (windowsReserved(name)) return 'This name is reserved by Windows. Choose another.'
  return null
}

function windowsReserved(name: string): boolean {
  const base = name.toLowerCase().split('.')[0]
  return ['con', 'prn', 'aux', 'nul'].includes(base) || /^(com|lpt)[1-9]$/.test(base)
}

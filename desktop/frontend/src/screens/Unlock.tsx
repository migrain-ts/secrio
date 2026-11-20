import { type FormEvent, useMemo, useRef, useState } from 'react'
import { ApiError } from '@/api/errors'
import { InfoCircleIcon, WarningCircleIcon } from '@/components/icons'
import { Spinner } from '@/components/motion/spinner'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Field, fieldDescribedBy } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { FormScreen } from '@/screens/FormScreen'
import { useApp } from '@/state/AppProvider'
import type { Notice } from '@/state/machine'

const NOTICE: Record<Notice, string> = {
  interrupted: 'Your vault was locked while you were working. Your last change was not saved.',
  locked: 'Your vault was locked.',
}

export function Unlock({ vaultId, notice }: { vaultId: string | null; notice?: Notice }) {
  const { api, vaults, dispatch, syncVault } = useApp()
  // The preselected vault first, then the others by name.
  const ordered = useMemo(
    () => [...vaults.filter((v) => v.id === vaultId), ...vaults.filter((v) => v.id !== vaultId)],
    [vaults, vaultId],
  )
  const [chosen, setChosen] = useState<string | null>(vaultId)
  const target = chosen ?? (vaults.length === 1 ? vaults[0].id : null)
  // The password lives only here, and is cleared the moment it is submitted.
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)
  const [passwordError, setPasswordError] = useState<string | null>(null)
  const [vaultError, setVaultError] = useState<string | null>(null)
  const [formError, setFormError] = useState<string | null>(null)
  const passwordRef = useRef<HTMLInputElement>(null)

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    if (busy) return
    setPasswordError(null)
    setVaultError(null)
    setFormError(null)
    if (!target) {
      // nothing was submitted: keep the typed password, point at the select
      setVaultError('Choose a vault.')
      document.getElementById('unlock-vault')?.focus()
      return
    }
    const typed = password
    setPassword('')
    if (typed === '') {
      setPasswordError('Enter your password.')
      passwordRef.current?.focus()
      return
    }
    setBusy(true)
    try {
      await api.unlock(target, typed) // selects the vault on success
      await syncVault()
      dispatch({ type: 'UNLOCKED', vaultId: target })
    } catch (err) {
      setBusy(false)
      // One generic message whatever the cause (ux: protect secrets).
      if (err instanceof ApiError && err.code === 'invalid_password') setPasswordError('Wrong password')
      else setFormError("Couldn't unlock the vault. Try again.")
      passwordRef.current?.focus()
    }
  }

  const single = vaults.length === 1 ? vaults[0] : null
  const description = single ? (
    <>
      Enter the password for <span className="font-medium text-foreground">{single.name}</span>.
    </>
  ) : (
    'Choose a vault and enter its password.'
  )

  return (
    <FormScreen title="Unlock your vault" description={description}>
      {notice && (
        <p role="status" className="mb-4 flex items-center gap-2 text-sm text-muted-foreground">
          <InfoCircleIcon aria-hidden className="size-4 shrink-0" />
          {NOTICE[notice]}
        </p>
      )}
      <form className="flex flex-col gap-4" onSubmit={onSubmit} noValidate aria-busy={busy}>
        {formError && (
          <Alert variant="destructive">
            <WarningCircleIcon aria-hidden className="size-4" />
            <AlertDescription>{formError}</AlertDescription>
          </Alert>
        )}
        {vaults.length > 1 && (
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="unlock-vault">Vault</Label>
            <Select
              value={target}
              onValueChange={(v) => {
                setChosen(v as string)
                setVaultError(null)
              }}
              items={ordered.map((v) => ({ value: v.id, label: v.name }))}
            >
              <SelectTrigger
                id="unlock-vault"
                aria-invalid={vaultError ? true : undefined}
                aria-describedby={vaultError ? 'unlock-vault-error' : undefined}
              >
                <SelectValue placeholder="Choose a vault" />
              </SelectTrigger>
              <SelectContent>
                {ordered.map((v) => (
                  <SelectItem key={v.id} value={v.id}>
                    {v.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {vaultError && (
              <p id="unlock-vault-error" className="text-xs text-destructive-text">
                {vaultError}
              </p>
            )}
          </div>
        )}
        <Field label="Password" htmlFor="unlock-password" error={passwordError ?? undefined} announce>
          <Input
            ref={passwordRef}
            id="unlock-password"
            type="password"
            autoComplete="off"
            spellCheck={false}
            autoCapitalize="off"
            autoCorrect="off"
            autoFocus
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            aria-invalid={passwordError ? true : undefined}
            aria-describedby={fieldDescribedBy('unlock-password', passwordError ?? undefined)}
          />
        </Field>
        {/* Stays enabled until submission starts, then disabled with its label kept. */}
        <Button type="submit" className="w-full" disabled={busy}>
          {busy && <Spinner label={null} className="size-4" />}
          Unlock
        </Button>
        <Button
          type="button"
          variant="ghost"
          className="w-full"
          disabled={busy}
          onClick={() => dispatch({ type: 'GOTO_CREATE_VAULT' })}
        >
          Create a new vault
        </Button>
      </form>
    </FormScreen>
  )
}

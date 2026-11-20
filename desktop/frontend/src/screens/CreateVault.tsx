import { type FormEvent, useRef, useState } from 'react'
import { ApiError } from '@/api/errors'
import { WarningCircleIcon } from '@/components/icons'
import { Spinner } from '@/components/motion/spinner'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Field, fieldDescribedBy } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { VAULT_NAME_HINT, vaultNameError } from '@/lib/vault-name'
import { FormScreen } from '@/screens/FormScreen'
import { useApp } from '@/state/AppProvider'

type FieldName = 'name' | 'password' | 'confirm'
type Errors = Partial<Record<FieldName, string>>

function validate(name: string, password: string, confirm: string): Errors {
  const e: Errors = {}
  const n = vaultNameError(name)
  if (n) e.name = n
  if (password === '') e.password = 'Enter a password.'
  else if (confirm !== password) e.confirm = "Passwords don't match."
  return e
}

const ORDER: FieldName[] = ['name', 'password', 'confirm']
const SERVER_FIELD: Partial<Record<string, [FieldName, string]>> = {
  duplicate: ['name', 'A vault with this name already exists.'],
  invalid_name: ['name', "This name isn't allowed. Choose another."],
  invalid_value: ['password', 'Enter a password.'],
}

export function CreateVault() {
  const { api, vaults, dispatch, syncVault } = useApp()
  const firstRun = vaults.length === 0
  const [name, setName] = useState('')
  // Passwords live only here, and are cleared the moment they are submitted.
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  // Errors are shown per field after blur or submit, never while typing the
  // first characters (ux: validate on the user's terms).
  const [touched, setTouched] = useState<Partial<Record<FieldName, boolean>>>({})
  const [serverErrors, setServerErrors] = useState<Errors>({})
  const [formError, setFormError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const refs = {
    name: useRef<HTMLInputElement>(null),
    password: useRef<HTMLInputElement>(null),
    confirm: useRef<HTMLInputElement>(null),
  }

  const clientErrors = validate(name, password, confirm)
  const shown = (f: FieldName) => serverErrors[f] ?? (touched[f] ? clientErrors[f] : undefined)
  const blur = (f: FieldName) => setTouched((t) => ({ ...t, [f]: true }))

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    if (busy) return
    setTouched({ name: true, password: true, confirm: true })
    setServerErrors({})
    setFormError(null)
    const first = ORDER.find((f) => clientErrors[f])
    if (first) {
      refs[first].current?.focus()
      return
    }
    const typed = password
    setPassword('')
    setConfirm('')
    setBusy(true)
    try {
      await api.createVault(name, typed) // creates, unlocks and selects it
      await syncVault()
      dispatch({ type: 'VAULT_CREATED' })
    } catch (err) {
      setBusy(false)
      // The cleared passwords must be typed again: their checks restart.
      setTouched({ name: true })
      const mapped = err instanceof ApiError ? SERVER_FIELD[err.code] : undefined
      if (mapped) {
        setServerErrors({ [mapped[0]]: mapped[1] })
        refs[mapped[0]].current?.focus()
      } else {
        setFormError("Couldn't create the vault. Try again.")
      }
    }
  }

  const input = (f: FieldName) => ({
    id: `create-${f}`,
    ref: refs[f],
    autoComplete: 'off',
    spellCheck: false,
    autoCapitalize: 'off',
    autoCorrect: 'off',
    onBlur: () => blur(f),
    'aria-invalid': shown(f) ? true : undefined,
    'aria-describedby': fieldDescribedBy(`create-${f}`, shown(f), f === 'name' ? VAULT_NAME_HINT : undefined),
  })

  return (
    <FormScreen
      title={firstRun ? 'Create your first vault' : 'Create a vault'}
      description="It stays on this computer, encrypted with your password."
    >
      <form className="flex flex-col gap-4" onSubmit={onSubmit} noValidate aria-busy={busy}>
        {formError && (
          <Alert variant="destructive">
            <WarningCircleIcon aria-hidden className="size-4" />
            <AlertDescription>{formError}</AlertDescription>
          </Alert>
        )}
        <Field label="Vault name" htmlFor="create-name" description={VAULT_NAME_HINT} error={shown('name')}>
          <Input
            {...input('name')}
            autoFocus
            value={name}
            onChange={(e) => {
              setName(e.target.value)
              setServerErrors((s) => ({ ...s, name: undefined }))
            }}
          />
        </Field>
        <Field label="Password" htmlFor="create-password" error={shown('password')}>
          <Input
            {...input('password')}
            type="password"
            value={password}
            onChange={(e) => {
              setPassword(e.target.value)
              setServerErrors((s) => ({ ...s, password: undefined }))
            }}
          />
        </Field>
        <Field label="Confirm password" htmlFor="create-confirm" error={shown('confirm')}>
          <Input {...input('confirm')} type="password" value={confirm} onChange={(e) => setConfirm(e.target.value)} />
        </Field>
        <Button type="submit" className="w-full" disabled={busy}>
          {busy && <Spinner label={null} className="size-4" />}
          Create vault
        </Button>
        {!firstRun && (
          <Button
            type="button"
            variant="ghost"
            className="w-full"
            onClick={() => dispatch({ type: 'GOTO_UNLOCK', vaultId: null })}
          >
            Back to unlock
          </Button>
        )}
      </form>
    </FormScreen>
  )
}

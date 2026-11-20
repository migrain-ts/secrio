// Create or edit one secret. The typed value lives only in this dialog's body
// state: the body unmounts when the dialog closes, so Cancel, Escape, a
// successful save and a lock (the screen unmounts) all drop it.
import { type FormEvent, type RefObject, useEffect, useRef, useState } from 'react'
import { ApiError } from '@/api/errors'
import { SECRET_TYPES, type SecretDTO, type SecretType } from '@/api/types'
import { EyeClosedIcon, EyeIcon, WarningCircleIcon } from '@/components/icons'
import { Spinner } from '@/components/motion/spinner'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Dialog, DialogClose, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Field, fieldDescribedBy } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { TYPE_LABELS } from '@/lib/secret-types'
import { cn } from '@/lib/utils'
import { useApp } from '@/state/AppProvider'

const NAME_RE = /^[A-Za-z_][A-Za-z0-9_]*$/
const MAX_VALUE_BYTES = 1 << 20
const NAME_ERROR = "Use letters, digits and underscores; don't start with a digit"

const nameError = (n: string) => (NAME_RE.test(n) ? null : NAME_ERROR)
function valueError(v: string): string | null {
  if (v === '') return 'A value is required'
  if (new TextEncoder().encode(v).length > MAX_VALUE_BYTES) return 'Value is too large (max 1 MiB)'
  return null
}

const TYPE_ITEMS = SECRET_TYPES.map((t) => ({ value: t, label: TYPE_LABELS[t] }))

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  envId: string
  /** The secret to edit; null creates a new one. */
  editing: SecretDTO | null
  /** For the duplicate-name message. */
  envName: string
  /** Names the screen has loaded for this env: create refuses them (case-sensitive). */
  existingNames: readonly string[]
  /** After a successful save, with the saved name. */
  onSaved: (name: string) => void
  /** The environment no longer exists (not_found): the parent refreshes. */
  onEnvGone: () => void
  /** Where focus goes on close (default: the element focused before opening). */
  finalFocus?: RefObject<HTMLElement | null> | (() => HTMLElement | null)
}

export function SecretForm({ open, onOpenChange, finalFocus, ...rest }: Props) {
  // Blocks Escape/Cancel while a save is in flight.
  const busyRef = useRef(false)
  const nameRef = useRef<HTMLInputElement>(null)
  const valueRef = useRef<HTMLTextAreaElement>(null)
  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (next) onOpenChange(true)
        else if (!busyRef.current) onOpenChange(false)
      }}
    >
      <DialogContent
        initialFocus={rest.editing ? valueRef : nameRef}
        finalFocus={finalFocus}
        className="max-w-[32rem]"
      >
        <Body {...rest} busyRef={busyRef} nameRef={nameRef} valueRef={valueRef} close={() => onOpenChange(false)} />
      </DialogContent>
    </Dialog>
  )
}

function Body({
  envId,
  editing,
  envName,
  existingNames,
  onSaved,
  onEnvGone,
  busyRef,
  nameRef,
  valueRef,
  close,
}: Omit<Props, 'open' | 'onOpenChange' | 'finalFocus'> & {
  busyRef: RefObject<boolean>
  nameRef: RefObject<HTMLInputElement | null>
  valueRef: RefObject<HTMLTextAreaElement | null>
  close: () => void
}) {
  const { api } = useApp()
  const [name, setName] = useState(editing?.name ?? '')
  const [value, setValue] = useState('')
  const [type, setType] = useState<SecretType>(editing?.type ?? 'text')
  const [shown, setShown] = useState(false)
  const [busy, setBusy] = useState(false)
  // Edit: the facade needs the value on save, so load the current one.
  const [loading, setLoading] = useState(editing !== null)
  const [loadFailed, setLoadFailed] = useState(false)
  const [nameErr, setNameErr] = useState<string | null>(null)

  // Create never overwrites: SaveSecret creates OR updates, so an existing
  // name (exact match, names are case-sensitive) is refused here.
  const dupError = `A secret with this name already exists in ${envName}`
  const createNameError = (n: string) => nameError(n) ?? (existingNames.includes(n) ? dupError : null)
  const [valueErr, setValueErr] = useState<string | null>(null)
  const [formErr, setFormErr] = useState<string | null>(null)

  useEffect(() => {
    if (!editing) return
    let live = true
    api.revealSecret(envId, editing.name).then(
      (v) => {
        if (!live) return
        setValue(v)
        setLoading(false)
        // after the re-render that enables the field
        setTimeout(() => valueRef.current?.focus())
      },
      () => {
        if (!live) return
        setLoading(false)
        setLoadFailed(true)
      },
    )
    return () => {
      live = false
    }
  }, [api, envId, editing, valueRef])

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    if (busyRef.current || loading || loadFailed) return
    setFormErr(null)
    const ne = editing ? null : createNameError(name)
    const ve = valueError(value)
    setNameErr(ne)
    setValueErr(ve)
    if (ne) return nameRef.current?.focus()
    if (ve) return valueRef.current?.focus()
    busyRef.current = true
    setBusy(true)
    try {
      if (!editing) {
        // Re-check against a fresh list: the name may have been created
        // elsewhere (the CLI) since the screen loaded its list.
        let fresh: SecretDTO[]
        try {
          fresh = await api.listSecrets(envId)
        } catch (err) {
          // locked/no_vault: the guarded api already left the screen.
          if (!(err instanceof ApiError && (err.code === 'locked' || err.code === 'no_vault'))) {
            setFormErr("Couldn't save the secret. Try again.")
          }
          return
        }
        if (fresh.some((s) => s.name === name)) {
          setNameErr(dupError)
          nameRef.current?.focus()
          return
        }
      }
      await api.saveSecret(envId, name, value, type)
      setValue('')
      busyRef.current = false
      close()
      onSaved(name)
    } catch (err) {
      // locked: the guarded api already left the screen (this unmounts).
      const code = err instanceof ApiError ? err.code : 'internal'
      if (code === 'invalid_name') {
        setNameErr(NAME_ERROR)
        nameRef.current?.focus()
      } else if (code === 'invalid_value') {
        setValueErr('This value is not accepted')
        valueRef.current?.focus()
      } else if (code === 'not_found') {
        setFormErr('This environment no longer exists')
        onEnvGone()
      } else {
        setFormErr("Couldn't save the secret. Try again.")
      }
    } finally {
      busyRef.current = false
      setBusy(false)
    }
  }

  const valueDescribedBy = valueErr ? fieldDescribedBy('secret-value', valueErr) : loading ? 'secret-value-loading' : undefined

  return (
    <form className="flex flex-col gap-4" onSubmit={onSubmit} noValidate aria-busy={busy}>
      <DialogHeader>
        <DialogTitle>{editing ? 'Edit secret' : 'New secret'}</DialogTitle>
      </DialogHeader>
      {(formErr || loadFailed) && (
        <Alert variant="destructive">
          <WarningCircleIcon aria-hidden className="size-4" />
          <AlertDescription>{formErr ?? "Couldn't load the current value."}</AlertDescription>
        </Alert>
      )}
      <Field label="Name" htmlFor="secret-name" error={nameErr ?? undefined}>
        <Input
          ref={nameRef}
          id="secret-name"
          className="font-mono"
          autoComplete="off"
          spellCheck={false}
          autoCapitalize="off"
          autoCorrect="off"
          disabled={editing !== null}
          value={name}
          onChange={(e) => setName(e.target.value)}
          onBlur={() => {
            if (!editing && name !== '') setNameErr(createNameError(name))
          }}
          aria-invalid={nameErr ? true : undefined}
          aria-describedby={fieldDescribedBy('secret-name', nameErr ?? undefined)}
        />
      </Field>
      <Field error={valueErr ?? undefined} htmlFor="secret-value">
        <div className="mb-1.5 flex items-center justify-between">
          <Label htmlFor="secret-value" className={cn('text-sm font-medium', valueErr && 'text-destructive')}>
            Value
          </Label>
          {/* constant name, state in aria-pressed */}
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="h-7 px-2"
            aria-label="Show value"
            aria-pressed={shown}
            onClick={() => setShown((s) => !s)}
          >
            {shown ? <EyeClosedIcon aria-hidden className="size-4" /> : <EyeIcon aria-hidden className="size-4" />}
            Show
          </Button>
        </div>
        <Textarea
          ref={valueRef}
          id="secret-value"
          rows={4}
          className={cn('font-mono break-all', !shown && '[-webkit-text-security:disc]')}
          autoComplete="off"
          spellCheck={false}
          autoCapitalize="off"
          autoCorrect="off"
          disabled={loading || loadFailed}
          value={value}
          // A value is exact (no trimming); only NUL, which no store accepts, is dropped.
          onChange={(e) => setValue(e.target.value.replaceAll('\0', ''))}
          onBlur={() => {
            if (value !== '') setValueErr(valueError(value))
          }}
          aria-invalid={valueErr ? true : undefined}
          aria-describedby={valueDescribedBy}
        />
        {loading && (
          <p id="secret-value-loading" role="status" className="mt-1.5 flex items-center gap-2 text-xs text-muted-foreground">
            <Spinner label={null} className="size-3" />
            Loading the current value
          </p>
        )}
      </Field>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="secret-type">Type</Label>
        <Select value={type} onValueChange={(v) => setType(v as SecretType)} items={TYPE_ITEMS}>
          <SelectTrigger id="secret-type" aria-label="Type">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {TYPE_ITEMS.map((t) => (
              <SelectItem key={t.value} value={t.value}>
                {t.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <DialogFooter className="gap-2 sm:gap-0">
        <DialogClose render={<Button type="button" variant="ghost" />}>Cancel</DialogClose>
        <Button type="submit" disabled={busy || loading || loadFailed}>
          {busy && <Spinner label={null} className="size-4" />}
          Save
        </Button>
      </DialogFooter>
    </form>
  )
}

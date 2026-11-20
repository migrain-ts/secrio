import { type FormEvent, type RefObject, useRef, useState } from 'react'
import { ApiError } from '@/api/errors'
import type { EnvDTO, ErrorCode } from '@/api/types'
import { WarningCircleIcon } from '@/components/icons'
import { Spinner } from '@/components/motion/spinner'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Dialog, DialogClose, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Field, fieldDescribedBy } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { useApp } from '@/state/AppProvider'

/** Field-level copy per server code; anything else is a form-level alert. */
const FIELD_ERRORS: Partial<Record<ErrorCode, string>> = {
  duplicate: 'An environment with this name already exists',
  invalid_name: 'Use a name without control characters',
}

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  projectId: string
  onCreated: (env: EnvDTO) => void
  /** Where focus goes on close (default: the element focused before opening). */
  finalFocus?: RefObject<HTMLElement | null>
}

/** Same shape as NewProject: one name field, sent exactly as typed. */
export function NewEnv({ open, onOpenChange, projectId, onCreated, finalFocus }: Props) {
  const { api } = useApp()
  const [name, setName] = useState('')
  const [busy, setBusy] = useState(false)
  const [nameError, setNameError] = useState<string | null>(null)
  const [formError, setFormError] = useState(false)
  const busyRef = useRef(false)
  const inputRef = useRef<HTMLInputElement>(null)

  function close() {
    setName('')
    setNameError(null)
    setFormError(false)
    onOpenChange(false)
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    if (busyRef.current) return
    setNameError(null)
    setFormError(false)
    if (name === '') {
      setNameError('Enter an environment name')
      inputRef.current?.focus()
      return
    }
    busyRef.current = true
    setBusy(true)
    try {
      const env = await api.createEnvironment(projectId, name)
      close()
      onCreated(env)
    } catch (err) {
      const field = err instanceof ApiError ? FIELD_ERRORS[err.code] : undefined
      if (field) setNameError(field)
      else setFormError(true)
      inputRef.current?.focus()
    } finally {
      busyRef.current = false
      setBusy(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (next) onOpenChange(true)
        else if (!busyRef.current) close()
      }}
    >
      <DialogContent initialFocus={inputRef} finalFocus={finalFocus} className="max-w-[27.5rem]">
        <form className="flex flex-col gap-4" onSubmit={onSubmit} noValidate aria-busy={busy}>
          <DialogHeader>
            <DialogTitle>New environment</DialogTitle>
          </DialogHeader>
          {formError && (
            <Alert variant="destructive">
              <WarningCircleIcon aria-hidden className="size-4" />
              <AlertDescription>Couldn't create the environment. Try again.</AlertDescription>
            </Alert>
          )}
          <Field label="Name" htmlFor="new-env-name" error={nameError ?? undefined}>
            <Input
              ref={inputRef}
              id="new-env-name"
              placeholder="Staging"
              autoComplete="off"
              spellCheck={false}
              autoCapitalize="off"
              autoCorrect="off"
              value={name}
              onChange={(e) => setName(e.target.value)}
              aria-invalid={nameError ? true : undefined}
              aria-describedby={fieldDescribedBy('new-env-name', nameError ?? undefined)}
            />
          </Field>
          <DialogFooter className="gap-2 sm:gap-0">
            <DialogClose render={<Button type="button" variant="ghost" />}>Cancel</DialogClose>
            <Button type="submit" disabled={busy}>
              {busy && <Spinner label={null} className="size-4" />}
              Create
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

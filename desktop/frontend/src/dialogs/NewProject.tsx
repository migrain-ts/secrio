import { type FormEvent, type RefObject, useRef, useState } from 'react'
import { ApiError } from '@/api/errors'
import type { ErrorCode, ProjectDTO } from '@/api/types'
import { WarningCircleIcon } from '@/components/icons'
import { Spinner } from '@/components/motion/spinner'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Field, fieldDescribedBy } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { useApp } from '@/state/AppProvider'

/** Field-level copy per server code; anything else is a form-level alert. */
const FIELD_ERRORS: Partial<Record<ErrorCode, string>> = {
  duplicate: 'A project with this name already exists',
  invalid_name: 'Use a name without control characters',
}

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated: (project: ProjectDTO) => void
  /** Where focus goes on close (default: the element focused before opening). */
  finalFocus?: RefObject<HTMLElement | null>
}

export function NewProject({ open, onOpenChange, onCreated, finalFocus }: Props) {
  const { api } = useApp()
  const [name, setName] = useState('')
  const [busy, setBusy] = useState(false)
  const [nameError, setNameError] = useState<string | null>(null)
  const [formError, setFormError] = useState(false)
  const busyRef = useRef(false) // a second Enter lands before the re-render
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
    // Only an empty name is refused here: the name is sent exactly as typed,
    // the server decides what a valid name is.
    if (name === '') {
      setNameError('Enter a project name')
      inputRef.current?.focus()
      return
    }
    busyRef.current = true
    setBusy(true)
    try {
      const project = await api.createProject(name)
      close()
      onCreated(project)
    } catch (err) {
      // locked/no_vault: the guarded api already left this screen.
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
      {/* initialFocus, not autoFocus: autoFocus would focus before Base UI
          records the trigger, and focus would not return to it on close. */}
      <DialogContent initialFocus={inputRef} finalFocus={finalFocus} className="max-w-[27.5rem]">
        <form className="flex flex-col gap-4" onSubmit={onSubmit} noValidate aria-busy={busy}>
          <DialogHeader>
            <DialogTitle>New project</DialogTitle>
          </DialogHeader>
          {formError && (
            <Alert variant="destructive">
              <WarningCircleIcon aria-hidden className="size-4" />
              <AlertDescription>Couldn't create the project. Try again.</AlertDescription>
            </Alert>
          )}
          <Field label="Name" htmlFor="new-project-name" error={nameError ?? undefined}>
            <Input
              ref={inputRef}
              id="new-project-name"
              autoComplete="off"
              spellCheck={false}
              autoCapitalize="off"
              autoCorrect="off"
              value={name}
              onChange={(e) => setName(e.target.value)}
              aria-invalid={nameError ? true : undefined}
              aria-describedby={fieldDescribedBy('new-project-name', nameError ?? undefined)}
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

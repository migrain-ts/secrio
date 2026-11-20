import { type FormEvent, type ReactNode, type RefObject, useRef, useState } from 'react'
import { WarningCircleIcon } from '@/components/icons'
import { Spinner } from '@/components/motion/spinner'
import { Alert, AlertDescription } from '@/components/ui/alert'
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  /** Names the target and the consequence (ux: destructive copy). */
  description: ReactNode
  /** Verb + object, e.g. "Delete project". */
  confirmLabel: string
  /** Type-to-confirm: the confirm button waits for exactly this text. */
  requireText?: string
  /** Rejecting keeps the dialog open with an error; resolving closes it. */
  onConfirm: () => Promise<void>
  /** Where focus goes on close (default: the element focused before opening).
   *  A function is read when focus is restored, after the closing render. */
  finalFocus?: RefObject<HTMLElement | null> | (() => HTMLElement | null)
}

/** Confirmation for an irreversible delete (projects, envs, secrets). */
export function ConfirmDelete({
  open,
  onOpenChange,
  title,
  description,
  confirmLabel,
  requireText,
  onConfirm,
  finalFocus,
}: Props) {
  const [typed, setTyped] = useState('')
  const [busy, setBusy] = useState(false)
  const [failed, setFailed] = useState(false)
  // A ref as well as state: a double click lands before the re-render.
  const busyRef = useRef(false)
  const cancelRef = useRef<HTMLButtonElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)
  const confirmRef = useRef<HTMLButtonElement>(null)
  const ready = requireText === undefined || typed === requireText

  function close() {
    setTyped('')
    setFailed(false)
    onOpenChange(false)
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    if (!ready || busyRef.current) return
    busyRef.current = true
    setBusy(true)
    setFailed(false)
    try {
      await onConfirm()
      close()
    } catch {
      setFailed(true) // fixed copy: never the raw error
      // the disabled button dropped focus: back to where the user acts
      setTimeout(() => (requireText === undefined ? confirmRef : inputRef).current?.focus())
    } finally {
      busyRef.current = false
      setBusy(false)
    }
  }

  return (
    <AlertDialog
      open={open}
      onOpenChange={(next) => {
        if (next) onOpenChange(true)
        else if (!busyRef.current) close()
      }}
    >
      {/* Safe default: Cancel, unless the user must type the name first. */}
      <AlertDialogContent initialFocus={requireText === undefined ? cancelRef : inputRef} finalFocus={finalFocus}>
        <form className="flex flex-col gap-4" onSubmit={onSubmit} noValidate aria-busy={busy}>
          <AlertDialogHeader>
            <AlertDialogTitle>{title}</AlertDialogTitle>
            <AlertDialogDescription>{description}</AlertDialogDescription>
          </AlertDialogHeader>
          {failed && (
            <Alert variant="destructive">
              <WarningCircleIcon aria-hidden className="size-4" />
              <AlertDescription>Couldn't delete. Try again.</AlertDescription>
            </Alert>
          )}
          {requireText !== undefined && (
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="confirm-delete-text" className="text-sm text-muted-foreground">
                {/* quoted, spaces kept: a name may start or end with a space */}
                Type <span className="font-medium whitespace-pre-wrap text-foreground">“{requireText}”</span> to confirm
              </Label>
              <Input
                ref={inputRef}
                id="confirm-delete-text"
                autoComplete="off"
                spellCheck={false}
                autoCapitalize="off"
                autoCorrect="off"
                value={typed}
                onChange={(e) => setTyped(e.target.value)}
              />
            </div>
          )}
          <AlertDialogFooter>
            <AlertDialogCancel ref={cancelRef} type="button">
              Cancel
            </AlertDialogCancel>
            <Button ref={confirmRef} type="submit" variant="destructive" disabled={!ready || busy}>
              {busy && <Spinner label={null} className="size-4" />}
              {confirmLabel}
            </Button>
          </AlertDialogFooter>
        </form>
      </AlertDialogContent>
    </AlertDialog>
  )
}

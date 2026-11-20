// Import a .env file into one env: pick, preview the names, choose what to do
// with existing ones, apply. The facade returns names only, so no value ever
// reaches this dialog. Every async step carries a request id: a late answer
// after another pick, a close or an unmount is dropped.
import { type RefObject, useEffect, useReducer, useRef } from 'react'
import { ApiError } from '@/api/errors'
import type { ImportStrategy } from '@/api/types'
import { WarningCircleIcon } from '@/components/icons'
import { Spinner } from '@/components/motion/spinner'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Radio } from '@base-ui/react/radio'
import { RadioGroup } from '@/components/ui/radio-group'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  countsLine,
  ERROR_TEXT,
  errorKind,
  importCount,
  importLabel,
  importMachine,
  type ImportSummary,
  INITIAL,
  summary,
} from '@/dialogs/importMachine'
import { cn } from '@/lib/utils'
import { useApp } from '@/state/AppProvider'

/** Names shown per list; the rest is "and K more". */
export const MAX_LISTED = 50

/** locked/no_vault: the guarded api already left the screen. */
const leftScreen = (e: unknown) => e instanceof ApiError && (e.code === 'locked' || e.code === 'no_vault')

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  envId: string
  envName: string
  onImported: (s: ImportSummary) => void
  /** Where focus goes on close; a function is read after the closing render. */
  finalFocus?: RefObject<HTMLElement | null> | (() => HTMLElement | null)
}

export function ImportEnv({ open, onOpenChange, envId, envName, onImported, finalFocus }: Props) {
  const { api } = useApp()
  const [s, dispatch] = useReducer(importMachine, INITIAL)
  const req = useRef(0) // the only request whose answer may land
  const applyingRef = useRef(false) // a double click lands before the re-render
  const primaryRef = useRef<HTMLButtonElement>(null)
  const strategyRef = useRef<HTMLFieldSetElement>(null)

  useEffect(
    () => () => {
      req.current++
    },
    [],
  )

  // Focus the primary action of each step the user acts on. After a conflict
  // with existing names, focus the (reset) strategy instead: Enter must not be
  // one key away from an overwrite the user has not reviewed.
  const { step } = s
  const conflictChoice = step === 'preview' && s.error === 'conflict' && (s.preview?.duplicate.length ?? 0) > 0
  useEffect(() => {
    if (!open || !(step === 'idle' || step === 'preview' || step === 'error')) return
    const checked = conflictChoice ? strategyRef.current?.querySelector<HTMLElement>('[role="radio"][aria-checked="true"]') : null
    ;(checked ?? primaryRef.current)?.focus()
  }, [open, step, conflictChoice])

  function close() {
    req.current++
    applyingRef.current = false
    dispatch({ type: 'RESET' })
    onOpenChange(false)
  }

  async function runPreview(path: string) {
    const id = ++req.current
    try {
      const preview = await api.previewImport(envId, path)
      if (id === req.current) dispatch({ type: 'PREVIEWED', preview })
    } catch (e) {
      if (id === req.current && !leftScreen(e)) dispatch({ type: 'FAILED', error: errorKind(e) })
    }
  }

  async function pick() {
    if (applyingRef.current) return
    const id = ++req.current
    dispatch({ type: 'PICK' })
    try {
      const path = await api.pickEnvFile()
      if (id !== req.current) return
      if (path === '') return dispatch({ type: 'PICK_CANCELLED' })
      dispatch({ type: 'PICKED', path })
      await runPreview(path)
    } catch (e) {
      if (id === req.current && !leftScreen(e)) dispatch({ type: 'FAILED', error: errorKind(e) })
    }
  }

  function retry() {
    if (s.path === null) return void pick()
    dispatch({ type: 'REPREVIEW' })
    void runPreview(s.path)
  }

  async function apply() {
    const { path, preview, strategy } = s
    if (applyingRef.current || s.step !== 'preview' || !path || !preview) return
    if (importCount(preview, strategy) === 0) return close()
    applyingRef.current = true
    const id = ++req.current
    dispatch({ type: 'APPLY' })
    try {
      await api.applyImport(envId, path, strategy)
      if (id !== req.current) return
      close()
      onImported(summary(preview, strategy))
    } catch (e) {
      if (id !== req.current || leftScreen(e)) return
      const error = errorKind(e)
      dispatch({ type: 'FAILED', error })
      if (error === 'conflict') void runPreview(path) // same file, fresh counts
    } finally {
      applyingRef.current = false
    }
  }

  const busy = step === 'picking' || step === 'previewing' || step === 'applying'
  const applying = step === 'applying'
  const pv = step === 'preview' || applying ? s.preview : null
  const dupes = pv?.duplicate.length ?? 0
  const nothingToDo = pv !== null && importCount(pv, s.strategy) === 0

  let primary: { label: string; onClick: () => void; disabled: boolean }
  if (pv) primary = { label: importLabel(pv, s.strategy), onClick: () => void apply(), disabled: applying }
  else if (step === 'previewing') primary = { label: 'Import', onClick: () => {}, disabled: true }
  else if (step === 'error' && s.path !== null) primary = { label: 'Try again', onClick: retry, disabled: false }
  else primary = { label: 'Choose file', onClick: () => void pick(), disabled: step === 'picking' }

  const canPickAnother = pv !== null || step === 'previewing' || (step === 'error' && s.path !== null)
  const status = step === 'previewing' ? 'Reading the file' : applying ? 'Importing' : pv ? countsLine(pv) : ''

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (next) onOpenChange(true)
        else if (!applyingRef.current) close()
      }}
    >
      <DialogContent
        initialFocus={primaryRef}
        finalFocus={finalFocus}
        closeDisabled={applying}
        className="max-w-[32rem]"
      >
        <div className="flex min-w-0 flex-col gap-4" aria-busy={busy}>
          <DialogHeader>
            <DialogTitle>Import .env file</DialogTitle>
            {/* before a file is chosen; later the counts and the primary say it */}
            {step === 'idle' && <DialogDescription>Nothing changes until you confirm.</DialogDescription>}
          </DialogHeader>

          {s.error && (
            <Alert variant="destructive">
              <WarningCircleIcon aria-hidden className="size-4" />
              <AlertDescription>{ERROR_TEXT[s.error]}</AlertDescription>
            </Alert>
          )}

          {/* Announces the counts; on screen each list heading shows its own
              count, so the preview says it once (comprehension review). */}
          <p
            role="status"
            className={cn(
              'flex items-center gap-2 text-sm text-foreground tabular-nums empty:hidden',
              pv && !applying && pv.new.length + pv.duplicate.length > 0 && 'sr-only',
            )}
          >
            {(step === 'previewing' || applying) && <Spinner label={null} className="size-4" />}
            {status}
          </p>

          {pv && (
            <>
              {pv.new.length > 0 && <NameList id="import-new" title="Will be added" names={pv.new} />}
              {dupes > 0 && <NameList id="import-dupes" title={`Already in ${envName}`} names={pv.duplicate} />}
              {dupes > 0 && (
                <fieldset ref={strategyRef} className="flex flex-col gap-2">
                  <legend id="import-strategy" className="mb-2 text-sm font-medium text-foreground">
                    Existing secrets
                  </legend>
                  <RadioGroup
                    aria-labelledby="import-strategy"
                    value={s.strategy}
                    disabled={applying}
                    onValueChange={(v) => dispatch({ type: 'STRATEGY', strategy: v as ImportStrategy })}
                  >
                    <StrategyOption value="skip" label="Skip" />
                    <StrategyOption value="overwrite" label="Replace" />
                  </RadioGroup>
                </fieldset>
              )}
              {s.strategy === 'overwrite' && dupes > 0 && (
                <Alert>
                  <WarningCircleIcon aria-hidden className="size-4" />
                  <AlertDescription>
                    {dupes === 1
                      ? 'Replaces 1 value permanently.'
                      : `Replaces ${dupes} values permanently.`}
                  </AlertDescription>
                </Alert>
              )}
            </>
          )}

          <DialogFooter className="gap-2 sm:gap-0">
            <DialogClose render={<Button type="button" variant="ghost" disabled={applying} />}>Cancel</DialogClose>
            {canPickAnother && (
              <Button type="button" variant="outline" disabled={applying} onClick={() => void pick()}>
                Choose another file
              </Button>
            )}
            <Button
              ref={primaryRef}
              type="button"
              variant={nothingToDo ? 'secondary' : 'default'}
              disabled={primary.disabled}
              onClick={primary.onClick}
            >
              {(applying || step === 'picking') && <Spinner label={null} className="size-4" />}
              {primary.label}
            </Button>
          </DialogFooter>
        </div>
      </DialogContent>
    </Dialog>
  )
}

function NameList({ id, title, names }: { id: string; title: string; names: string[] }) {
  const rest = names.length - MAX_LISTED
  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex items-baseline gap-1.5 text-xs font-medium text-muted-foreground">
        <h3 id={id}>{title}</h3>
        {/* the status line announces the counts */}
        <span aria-hidden="true" className="tabular-nums">
          {names.length}
        </span>
      </div>
      <ScrollArea className="rounded-md bg-secondary/40 [&_[data-slot=scroll-area-viewport]]:max-h-36">
        <ul aria-labelledby={id} className="flex flex-col gap-1 px-3 py-2">
          {names.slice(0, MAX_LISTED).map((n) => (
            <li key={n} className="truncate font-mono text-sm text-foreground" title={n}>
              {n}
            </li>
          ))}
        </ul>
      </ScrollArea>
      {rest > 0 && <p className="text-xs text-muted-foreground tabular-nums">and {rest} more</p>}
    </div>
  )
}

/** The whole row is the radio (circle + label): a 32 px target, not the
 *  16 px circle alone (review gate). Named by its text. */
function StrategyOption({ value, label }: { value: ImportStrategy; label: string }) {
  return (
    <Radio.Root
      value={value}
      className="-mx-2 flex min-h-8 w-fit cursor-pointer items-center gap-2 rounded-md px-2 text-sm text-foreground outline-none hover:bg-secondary focus-visible:ring-2 focus-visible:ring-ring data-disabled:cursor-not-allowed data-disabled:opacity-50"
    >
      <span aria-hidden className="grid size-4 shrink-0 place-items-center rounded-full border border-primary">
        <Radio.Indicator className="size-2.5 rounded-full bg-primary" />
      </span>
      {label}
    </Radio.Root>
  )
}

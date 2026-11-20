// The secrets of one env: metadata only. The Value column is ALWAYS the same
// run of dots, never derived from (or holding) a real value.
import { type KeyboardEvent, memo, type RefObject, useCallback, useEffect, useRef } from 'react'
import type { SecretDTO } from '@/api/types'
import { CopyIcon, EditPencilIcon, EyeClosedIcon, EyeIcon, TrashIcon } from '@/components/icons'
import { Badge } from '@/components/ui/badge'
import { IconButton } from '@/components/ui/icon-button'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { MASK, shortDate, TYPE_LABELS } from '@/lib/secret-types'
import { cn } from '@/lib/utils'

export type RowAction = 'reveal' | 'copy' | 'edit' | 'delete'

interface Props {
  envName: string
  secrets: SecretDTO[]
  selected: string | null
  onSelect: (name: string) => void
  onAction: (action: RowAction, secret: SecretDTO, trigger: HTMLElement) => void
  /** The tbody (the parent focuses a row after a delete). */
  bodyRef: RefObject<HTMLTableSectionElement | null>
  /** The secret whose value is revealed (its Reveal button reads "Hide"). */
  revealedName: string | null
}

export function SecretTable({ envName, secrets, selected, onSelect, onAction, bodyRef, revealedName }: Props) {
  // Roving tab stop: the selected row, else the first.
  const tabbable = secrets.some((s) => s.name === selected) ? selected : (secrets[0]?.name ?? null)

  // Stable handler (memoised rows keep it) reading the latest list.
  const latest = useRef({ secrets, onSelect })
  useEffect(() => {
    latest.current = { secrets, onSelect }
  })
  const onKeyDown = useCallback((e: KeyboardEvent<HTMLTableRowElement>, index: number) => {
    if (e.target !== e.currentTarget) return // keys on the row's buttons are theirs
    const { secrets: list, onSelect: select } = latest.current
    let to: number
    if (e.key === 'Enter' || e.key === ' ') to = index
    else if (e.key === 'ArrowDown') to = index + 1
    else if (e.key === 'ArrowUp') to = index - 1
    else if (e.key === 'Home') to = 0
    else if (e.key === 'End') to = list.length - 1
    else return
    e.preventDefault()
    if (to < 0 || to >= list.length) return
    select(list[to].name)
    if (to !== index) bodyRef.current?.rows[to]?.focus()
  }, [bodyRef])

  return (
    <Table aria-label={`Secrets in ${envName}`} className="table-fixed">
      <TableHeader className="sticky top-0 z-10 bg-background">
        <TableRow className="hover:bg-transparent">
          <TableHead scope="col" className="h-9 px-3">
            Name
          </TableHead>
          <TableHead scope="col" className="h-9 w-32 px-3">
            Value
          </TableHead>
          <TableHead scope="col" className="h-9 w-28 px-3">
            Type
          </TableHead>
          <TableHead scope="col" className="h-9 w-32 px-3">
            Updated
          </TableHead>
          <TableHead scope="col" className="h-9 w-[8.5rem] px-3">
            <span className="sr-only">Actions</span>
          </TableHead>
        </TableRow>
      </TableHeader>
      <TableBody ref={bodyRef}>
        {secrets.map((s, i) => (
          <SecretRow
            key={s.name}
            secret={s}
            index={i}
            selected={s.name === selected}
            tabbable={s.name === tabbable}
            revealed={s.name === revealedName}
            onSelect={onSelect}
            onAction={onAction}
            onKeyDown={onKeyDown}
          />
        ))}
      </TableBody>
    </Table>
  )
}

interface RowProps {
  secret: SecretDTO
  index: number
  selected: boolean
  tabbable: boolean
  revealed: boolean
  onSelect: Props['onSelect']
  onAction: Props['onAction']
  onKeyDown: (e: KeyboardEvent<HTMLTableRowElement>, index: number) => void
}

const ACTIONS: [RowAction, string, typeof EyeIcon][] = [
  ['reveal', 'Reveal', EyeIcon],
  ['copy', 'Copy', CopyIcon],
  ['edit', 'Edit', EditPencilIcon],
  ['delete', 'Delete', TrashIcon],
]

// Memoised: a 500-row env re-renders only the rows whose props change.
const SecretRow = memo(
  function SecretRow({ secret: s, index, selected, tabbable, revealed, onSelect, onAction, onKeyDown }: RowProps) {
    return (
      <TableRow
        tabIndex={tabbable ? 0 : -1}
        aria-current={selected ? 'true' : undefined}
        data-state={selected ? 'selected' : undefined}
        onClick={() => onSelect(s.name)}
        onKeyDown={(e) => onKeyDown(e, index)}
        className={cn(
          'cursor-default outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring',
          'hover:bg-secondary data-[state=selected]:bg-accent',
        )}
      >
        <TableCell className="truncate px-3 py-1.5 font-mono text-[0.8125rem] text-foreground" title={s.name}>
          {s.name}
        </TableCell>
        <TableCell className="px-3 py-1.5 text-muted-foreground">
          <span aria-hidden="true" className="tracking-widest">
            {MASK}
          </span>
          <span className="sr-only">Hidden</span>
        </TableCell>
        <TableCell className="px-3 py-1.5">
          <Badge>{TYPE_LABELS[s.type] ?? s.type}</Badge>
        </TableCell>
        <TableCell className="whitespace-nowrap px-3 py-1.5 text-muted-foreground tabular-nums">
          <time dateTime={s.updatedAt} title={s.updatedAt}>
            {shortDate(s.updatedAt)}
          </time>
        </TableCell>
        <TableCell className="px-2 py-1">
          <div className="flex justify-end gap-0.5">
            {ACTIONS.map(([action, label, Icon]) => (
              <IconButton
                key={action}
                size="xs"
                aria-label={`${action === 'reveal' && revealed ? 'Hide' : label} ${s.name}`}
                tabIndex={tabbable ? 0 : -1}
                onClick={(e) => onAction(action, s, e.currentTarget)}
                className={action === 'delete' ? 'hover:text-destructive' : undefined}
              >
                {action === 'reveal' && revealed ? (
                  <EyeClosedIcon aria-hidden className="size-4" />
                ) : (
                  <Icon aria-hidden className="size-4" />
                )}
              </IconButton>
            ))}
          </div>
        </TableCell>
      </TableRow>
    )
  },
)

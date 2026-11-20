// The selected secret. Its value is masked unless the user revealed it; the
// revealed text appears only here, in one block, never in the table.
import type { SecretDTO } from '@/api/types'
import { CopyIcon, EditPencilIcon, EyeClosedIcon, EyeIcon, TrashIcon } from '@/components/icons'
import { Spinner } from '@/components/motion/spinner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { REVEAL_MS, type Reveal } from '@/hooks/useReveal'
import { MASK, shortDate, TYPE_LABELS } from '@/lib/secret-types'

interface Props {
  secret: SecretDTO | null
  reveal: Reveal
  onCopy: (s: SecretDTO, trigger: HTMLElement) => void
  onEdit: (s: SecretDTO, trigger: HTMLElement) => void
  onDelete: (s: SecretDTO, trigger: HTMLElement) => void
}

export function SecretDetail({ secret: s, reveal, onCopy, onEdit, onDelete }: Props) {
  return (
    <aside aria-label="Secret details" className="w-72 shrink-0 overflow-y-auto border-l border-border p-4 xl:w-80">
      {s === null ? (
        <p className="pt-8 text-center text-sm text-muted-foreground">Select a secret to see its details.</p>
      ) : (
        <Card className="flex flex-col gap-4 p-4">
          {/* name + description as one block: the card keeps a single 16 px gap */}
          <div className="flex flex-col gap-1">
            <h2 className="break-all font-mono text-sm font-medium text-foreground">{s.name}</h2>
            {s.description && <p className="text-sm text-muted-foreground">{s.description}</p>}
          </div>

          <section className="flex flex-col gap-2">
            <h3 className="text-xs font-medium text-muted-foreground">Value</h3>
            {reveal.value !== null ? (
              <pre
                data-revealed
                className="max-h-60 overflow-auto select-all whitespace-pre-wrap break-all rounded-md border border-border bg-secondary px-3 py-2 font-mono text-[0.8125rem] text-foreground"
              >
                {reveal.value}
              </pre>
            ) : (
              <p className="rounded-md border border-border bg-secondary px-3 py-2 text-muted-foreground">
                <span aria-hidden="true" className="tracking-widest">
                  {MASK}
                </span>
                <span className="sr-only">Hidden</span>
              </p>
            )}
            <div className="flex gap-2">
              <Button
                variant="secondary"
                size="sm"
                aria-busy={reveal.pending}
                onClick={() => (reveal.value !== null ? reveal.hide() : void reveal.reveal())}
              >
                {reveal.pending ? (
                  <Spinner label={null} className="size-4" />
                ) : reveal.value !== null ? (
                  <EyeClosedIcon aria-hidden className="size-4" />
                ) : (
                  <EyeIcon aria-hidden className="size-4" />
                )}
                {reveal.value !== null ? 'Hide' : 'Reveal'}
              </Button>
              <Button variant="secondary" size="sm" onClick={(e) => onCopy(s, e.currentTarget)}>
                <CopyIcon aria-hidden className="size-4" />
                Copy
              </Button>
            </div>
            {reveal.value !== null && (
              <p className="text-xs text-muted-foreground">Hidden again after {REVEAL_MS / 1000} s.</p>
            )}
            {reveal.failed && (
              <p role="alert" className="text-xs text-destructive-text">
                Couldn't show the value.
              </p>
            )}
          </section>

          <dl className="grid grid-cols-[5rem_1fr] gap-x-3 gap-y-2 text-sm">
            <dt className="text-muted-foreground">Type</dt>
            <dd>
              <Badge>{TYPE_LABELS[s.type] ?? s.type}</Badge>
            </dd>
            <dt className="text-muted-foreground">Created</dt>
            <dd className="tabular-nums">
              <time dateTime={s.createdAt} title={s.createdAt}>
                {shortDate(s.createdAt)}
              </time>
            </dd>
            <dt className="text-muted-foreground">Updated</dt>
            <dd className="tabular-nums">
              <time dateTime={s.updatedAt} title={s.updatedAt}>
                {shortDate(s.updatedAt)}
              </time>
            </dd>
          </dl>

          <div className="flex gap-2 border-t border-border pt-4">
            <Button variant="secondary" size="sm" onClick={(e) => onEdit(s, e.currentTarget)}>
              <EditPencilIcon aria-hidden className="size-4" />
              Edit
            </Button>
            <Button variant="ghost" size="sm" className="hover:text-destructive-text" onClick={(e) => onDelete(s, e.currentTarget)}>
              <TrashIcon aria-hidden className="size-4" />
              Delete
            </Button>
          </div>
        </Card>
      )}
    </aside>
  )
}

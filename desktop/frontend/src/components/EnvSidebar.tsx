// The project's environments: the screen's navigation. The current env is
// marked with a fill (kit SidebarItem active) and aria-current.
import { type Ref, useRef } from 'react'
import type { EnvDTO } from '@/api/types'
import { MoreHorizIcon, PlusIcon, TrashIcon } from '@/components/icons'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { IconButton } from '@/components/ui/icon-button'
import { Sidebar, SidebarContent, SidebarItem } from '@/components/ui/sidebar'
import { Skeleton } from '@/components/ui/skeleton'

const plural = (n: number) => `${n} ${n === 1 ? 'secret' : 'secrets'}`

interface Props {
  /** null while loading. */
  envs: EnvDTO[] | null
  currentId: string | null
  onSelect: (envId: string) => void
  onNew: () => void
  onDelete: (env: EnvDTO, trigger: HTMLElement | null) => void
  newRef: Ref<HTMLButtonElement>
}

export function EnvSidebar({ envs, currentId, onSelect, onNew, onDelete, newRef }: Props) {
  return (
    <Sidebar className="w-56 shrink-0">
      <nav aria-label="Environments" className="flex min-h-0 flex-1 flex-col">
        <div className="flex h-11 items-center justify-between pl-4 pr-2">
          <h2 className="text-xs font-medium text-muted-foreground">Environments</h2>
          {/* in the no-env state the empty state carries this action instead */}
          {envs !== null && envs.length > 0 && (
            <IconButton ref={newRef} size="xs" aria-label="New environment" title="New environment" onClick={onNew}>
              <PlusIcon aria-hidden className="size-4" />
            </IconButton>
          )}
        </div>
        <SidebarContent className="px-2 pt-0">
          {envs === null ? (
            <div aria-hidden className="flex flex-col gap-1">
              {[0, 1, 2].map((i) => (
                <Skeleton key={i} className="h-9 rounded-lg" />
              ))}
            </div>
          ) : (
            <ul role="list" className="flex flex-col gap-0.5">
              {envs.map((e) => (
                <EnvItem
                  key={e.id}
                  env={e}
                  active={e.id === currentId}
                  onSelect={onSelect}
                  onDelete={onDelete}
                />
              ))}
            </ul>
          )}
        </SidebarContent>
      </nav>
    </Sidebar>
  )
}

function EnvItem({
  env,
  active,
  onSelect,
  onDelete,
}: {
  env: EnvDTO
  active: boolean
  onSelect: (id: string) => void
  onDelete: Props['onDelete']
}) {
  const triggerRef = useRef<HTMLButtonElement>(null)
  return (
    <li className="group relative">
      <SidebarItem
        label={env.name}
        active={active}
        aria-current={active ? 'page' : undefined}
        aria-label={`${env.name}, ${plural(env.secretCount)}`}
        title={env.name}
        onClick={() => onSelect(env.id)}
        className="h-9 pr-16"
      />
      {/* trailing slot: fixed width, right-aligned count */}
      <span
        aria-hidden
        className="pointer-events-none absolute right-9 top-1/2 w-6 -translate-y-1/2 text-right text-xs text-muted-foreground tabular-nums"
      >
        {env.secretCount}
      </span>
      <DropdownMenu>
        <DropdownMenuTrigger
          ref={triggerRef}
          render={<IconButton size="xs" className="absolute right-1 top-1/2 -translate-y-1/2" />}
          aria-label={`Environment actions for ${env.name}`}
        >
          <MoreHorizIcon aria-hidden className="size-4" />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onClick={() => onDelete(env, triggerRef.current)} className="gap-2 text-destructive">
            <TrashIcon aria-hidden className="size-4" />
            Delete environment
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </li>
  )
}

// The project picker: the screen after unlocking. One task: open a project
// (or create one). Delete hides behind each card's actions menu.
//
// Task 12 (project screen): when loading a project or its envs fails with
// `not_found` (e.g. a restored screen whose project was deleted from the CLI
// while locked), dispatch({ type: 'OPEN_PROJECTS' }) to come back here, where
// the fresh list no longer shows it. `locked`/`no_vault` need nothing: the
// guarded api in useApp() already left the screen.
import { useCallback, useEffect, useId, useRef, useState } from 'react'
import { ApiError } from '@/api/errors'
import type { ProjectDTO } from '@/api/types'
import { FolderIcon, MoreHorizIcon, PlusIcon, SearchIcon, TrashIcon, WarningCircleIcon } from '@/components/icons'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Empty } from '@/components/ui/empty'
import { IconButton } from '@/components/ui/icon-button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/use-toast'
import { ConfirmDelete } from '@/dialogs/ConfirmDelete'
import { NewProject } from '@/dialogs/NewProject'
import { useApp } from '@/state/AppProvider'

const plural = (n: number, word: string) => `${n} ${n === 1 ? word : `${word}s`}`

export function Projects() {
  const { api, dispatch } = useApp()
  const [projects, setProjects] = useState<ProjectDTO[] | null>(null)
  const [failed, setFailed] = useState(false)
  const [query, setQuery] = useState('')
  const [creating, setCreating] = useState(false)
  // The target outlives `deleteOpen` so the dialog keeps its text while it animates out.
  const [deleting, setDeleting] = useState<ProjectDTO | null>(null)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const newRef = useRef<HTMLButtonElement>(null)
  const searchRef = useRef<HTMLInputElement>(null)
  // Where focus goes when the delete dialog closes: the card's actions
  // button on cancel, "New project" after a delete (the card is gone; read
  // lazily because it may be the empty state's button by then).
  const menuTriggerRef = useRef<HTMLElement | null>(null)
  const deletedRef = useRef(false)

  // A refresh keeps the current list on screen until the new one arrives.
  // Only the latest request may land (a create and a delete can overlap).
  const req = useRef(0)
  const load = useCallback(async () => {
    const id = ++req.current
    try {
      const ps = await api.listProjects()
      if (id !== req.current) return
      setProjects(ps)
      setFailed(false)
    } catch (e) {
      if (id !== req.current) return
      // locked/no_vault: the guarded api already moved to another screen.
      if (e instanceof ApiError && (e.code === 'locked' || e.code === 'no_vault')) return
      setFailed(true)
    }
  }, [api])

  useEffect(() => {
    void load()
  }, [load])

  async function deleteProject(p: ProjectDTO) {
    await api.deleteProject(p.id) // a rejection keeps the dialog open
    deletedRef.current = true
    await load()
    toast({ title: 'Project deleted' })
  }

  const q = query.toLocaleLowerCase()
  const shown = projects?.filter((p) => p.name.toLocaleLowerCase().includes(q)) ?? []
  const empty = projects !== null && projects.length === 0

  const newButton = (
    <Button ref={newRef} onClick={() => setCreating(true)}>
      <PlusIcon aria-hidden className="size-4" />
      New project
    </Button>
  )

  return (
    <div className="mx-auto w-full max-w-[75rem] px-4 py-8">
      <div className="flex flex-wrap items-center gap-3">
        <h1 className="mr-auto text-xl font-semibold tracking-tight text-foreground">Projects</h1>
        {projects !== null && !empty && (
          <div className="relative w-64">
            <SearchIcon
              aria-hidden
              className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
            />
            <Input
              ref={searchRef}
              type="search"
              aria-label="Search projects"
              placeholder="Search projects"
              autoComplete="off"
              spellCheck={false}
              className="px-8"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Escape') setQuery('')
              }}
            />
          </div>
        )}
        {!empty && newButton}
      </div>

      <div className="mt-6">
        {failed && (
          <Alert variant="destructive" className="mb-4 flex items-center gap-3">
            <WarningCircleIcon aria-hidden className="size-4" />
            <AlertDescription className="mr-auto">Couldn't load projects.</AlertDescription>
            <Button
              variant="secondary"
              size="sm"
              onClick={() => {
                setFailed(false)
                void load()
              }}
            >
              Try again
            </Button>
          </Alert>
        )}

        {projects === null && !failed && (
          <div aria-busy="true">
            <p role="status" className="sr-only">
              Loading projects
            </p>
            <div className="grid grid-cols-[repeat(auto-fit,minmax(15rem,1fr))] gap-3">
              {[0, 1, 2].map((i) => (
                <Skeleton key={i} className="h-[4.5rem] rounded-lg" />
              ))}
            </div>
          </div>
        )}

        {empty && (
          <Empty
            icon={<FolderIcon aria-hidden />}
            title="No projects yet"
            description="Create a project to keep its secrets."
            action={newButton}
          />
        )}

        {projects !== null && !empty && shown.length === 0 && (
          <div className="flex flex-col items-center gap-2 py-12 text-center">
            <p className="text-sm text-muted-foreground">No projects match</p>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                setQuery('')
                searchRef.current?.focus()
              }}
            >
              Clear search
            </Button>
          </div>
        )}

        {shown.length > 0 && (
          // role="list": list-style:none drops the implicit role in WebKit
          <ul role="list" aria-label="Projects" className="grid grid-cols-[repeat(auto-fit,minmax(15rem,1fr))] gap-3">
            {shown.map((p) => (
              <ProjectCard
                key={p.id}
                project={p}
                onOpen={() => dispatch({ type: 'OPEN_PROJECT', projectId: p.id, envId: null })}
                onDelete={(trigger) => {
                  menuTriggerRef.current = trigger
                  deletedRef.current = false
                  setDeleting(p)
                  setDeleteOpen(true)
                }}
              />
            ))}
          </ul>
        )}
      </div>

      <NewProject
        open={creating}
        onOpenChange={setCreating}
        finalFocus={newRef}
        onCreated={() => {
          setQuery('') // an active search could hide the new project
          void load()
          toast({ title: 'Project created' })
        }}
      />
      <ConfirmDelete
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title="Delete this project?"
        description={
          deleting && (
            <>
              Deletes <span className="font-medium text-foreground">{deleting.name}</span>, its{' '}
              {plural(deleting.envCount, 'environment')} and {plural(deleting.secretCount, 'secret')}. This can't be
              undone.
            </>
          )
        }
        confirmLabel="Delete project"
        requireText={deleting?.name}
        onConfirm={() => (deleting ? deleteProject(deleting) : Promise.resolve())}
        finalFocus={() => (deletedRef.current ? newRef.current : menuTriggerRef.current)}
      />
    </div>
  )
}

/** One card: the whole card is the open button; the actions menu is a sibling
 *  control over its corner (never nested inside the button). */
function ProjectCard({
  project: p,
  onOpen,
  onDelete,
}: {
  project: ProjectDTO
  onOpen: () => void
  onDelete: (trigger: HTMLElement | null) => void
}) {
  const triggerRef = useRef<HTMLButtonElement>(null)
  const countsId = useId()
  return (
    <li className="min-w-0">
      <Card className="relative h-full">
        {/* named by the project, described by its counts */}
        <Button
          variant="ghost"
          onClick={onOpen}
          aria-label={p.name}
          aria-describedby={countsId}
          className="h-full w-full flex-col items-start justify-start gap-1 rounded-lg p-4 text-left whitespace-normal text-foreground hover:bg-accent hover:text-foreground active:scale-100"
        >
          <span data-project-name title={p.name} className="w-[calc(100%-2rem)] truncate text-sm font-medium">
            {p.name}
          </span>
          <span id={countsId} className="flex gap-3 text-xs text-muted-foreground tabular-nums">
            <span>{plural(p.envCount, 'environment')}</span>
            <span>{plural(p.secretCount, 'secret')}</span>
          </span>
        </Button>
        <DropdownMenu>
          <DropdownMenuTrigger
            ref={triggerRef}
            render={<IconButton size="xs" className="absolute right-2 top-2" />}
            aria-label={`Project actions for ${p.name}`}
          >
            <MoreHorizIcon aria-hidden className="size-4" />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onClick={() => onDelete(triggerRef.current)} className="gap-2 text-destructive-text">
              <TrashIcon aria-hidden className="size-4" />
              Delete project
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </Card>
    </li>
  )
}

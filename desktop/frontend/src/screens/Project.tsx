// One project: its envs on the left, the current env's secrets in the middle,
// the selected secret on the right. Values never enter this screen's state
// except through useReveal (one value, briefly) and the edit dialog.
import { useCallback, useEffect, useRef, useState } from 'react'
import { ApiError } from '@/api/errors'
import type { EnvDTO, SecretDTO } from '@/api/types'
import { EnvSidebar } from '@/components/EnvSidebar'
import { FolderIcon, KeyIcon, PlusIcon, SearchIcon, WarningCircleIcon } from '@/components/icons'
import { SecretDetail } from '@/components/SecretDetail'
import { type RowAction, SecretTable } from '@/components/SecretTable'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Empty } from '@/components/ui/empty'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/use-toast'
import { ConfirmDelete } from '@/dialogs/ConfirmDelete'
import { NewEnv } from '@/dialogs/NewEnv'
import { SecretForm } from '@/dialogs/SecretForm'
import { useReveal } from '@/hooks/useReveal'
import { useApp } from '@/state/AppProvider'

const plural = (n: number) => `${n} ${n === 1 ? 'secret' : 'secrets'}`

/** locked/no_vault: the guarded api already left the screen. */
const leftScreen = (e: unknown) => e instanceof ApiError && (e.code === 'locked' || e.code === 'no_vault')
const isNotFound = (e: unknown) => e instanceof ApiError && e.code === 'not_found'

export function Project({ projectId, envId }: { projectId: string; envId: string | null }) {
  const { api, dispatch } = useApp()
  const [projectName, setProjectName] = useState<string | null>(null)
  const [envs, setEnvs] = useState<EnvDTO[] | null>(null)
  const [envsFailed, setEnvsFailed] = useState(false)
  const [secrets, setSecrets] = useState<SecretDTO[] | null>(null)
  const [secretsFailed, setSecretsFailed] = useState(false)
  const [selected, setSelected] = useState<string | null>(null)
  const [query, setQuery] = useState('')

  // Dialogs. Targets outlive `open` so the text stays while closing.
  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<SecretDTO | null>(null)
  const [newEnvOpen, setNewEnvOpen] = useState(false)
  const [delSecret, setDelSecret] = useState<SecretDTO | null>(null)
  const [delSecretOpen, setDelSecretOpen] = useState(false)
  const [delEnv, setDelEnv] = useState<EnvDTO | null>(null)
  const [delEnvOpen, setDelEnvOpen] = useState(false)

  const newRef = useRef<HTMLButtonElement>(null) // "New secret" (toolbar or empty state)
  const newEnvRef = useRef<HTMLButtonElement>(null) // "New environment" (sidebar or empty state)
  const searchRef = useRef<HTMLInputElement>(null)
  const bodyRef = useRef<HTMLTableSectionElement>(null)
  const triggerRef = useRef<HTMLElement | null>(null) // what opened the current delete dialog
  const deletedRef = useRef(false)

  // Sequence guards: only the latest request of each list may land.
  const envReq = useRef(0)
  const secReq = useRef(0)
  useEffect(
    () => () => {
      envReq.current++
      secReq.current++
    },
    [],
  )

  const loadEnvs = useCallback(
    async (withProject = false) => {
      const id = ++envReq.current
      try {
        const [ps, es] = await Promise.all([
          withProject ? api.listProjects() : Promise.resolve(null),
          api.listEnvironments(projectId),
        ])
        if (id !== envReq.current) return
        if (ps) {
          const p = ps.find((x) => x.id === projectId)
          if (!p) return dispatch({ type: 'OPEN_PROJECTS' }) // deleted elsewhere
          setProjectName(p.name)
        }
        setEnvs(es)
        setEnvsFailed(false)
      } catch (e) {
        if (id !== envReq.current || leftScreen(e)) return
        if (isNotFound(e)) dispatch({ type: 'OPEN_PROJECTS' })
        else setEnvsFailed(true)
      }
    },
    [api, projectId, dispatch],
  )

  useEffect(() => {
    void loadEnvs(true)
  }, [loadEnvs])

  // The env shown: the requested one if it exists, else the first.
  const env = envs?.find((e) => e.id === envId) ?? null
  useEffect(() => {
    if (envs !== null && !envs.some((e) => e.id === envId)) dispatch({ type: 'SELECT_ENV', envId: envs[0]?.id ?? null })
  }, [envs, envId, dispatch])
  const currentEnvId = env?.id ?? null

  const loadSecrets = useCallback(
    async (eid: string) => {
      const id = ++secReq.current
      try {
        const ss = await api.listSecrets(eid)
        if (id !== secReq.current) return
        setSecrets(ss)
        setSecretsFailed(false)
      } catch (e) {
        if (id !== secReq.current || leftScreen(e)) return
        // the env is gone: the fresh env list moves to another one
        if (isNotFound(e)) void loadEnvs()
        else setSecretsFailed(true)
      }
    },
    [api, loadEnvs],
  )

  // A new env: fresh list, no selection (and so no revealed value).
  useEffect(() => {
    setSelected(null)
    setSecrets(null)
    setSecretsFailed(false)
    if (currentEnvId) void loadSecrets(currentEnvId)
    else secReq.current++
  }, [currentEnvId, loadSecrets])

  const refresh = useCallback(async () => {
    await Promise.all([currentEnvId ? loadSecrets(currentEnvId) : null, loadEnvs()])
  }, [currentEnvId, loadSecrets, loadEnvs])

  const selectedSecret = secrets?.find((s) => s.name === selected) ?? null
  const selName = selectedSecret?.name ?? null
  const reveal = useReveal(api, currentEnvId, selName, () => void refresh())

  // A row's Reveal on another row: select it first, reveal once selected.
  const pendingReveal = useRef<string | null>(null)
  const doReveal = reveal.reveal
  useEffect(() => {
    if (pendingReveal.current !== null && pendingReveal.current === selName) {
      pendingReveal.current = null
      void doReveal()
    }
  }, [selName, doReveal])

  /** After a list change removed the focused control, land on New secret. */
  const rescueFocus = () => {
    if (!document.activeElement || document.activeElement === document.body) newRef.current?.focus()
  }

  async function copy(s: SecretDTO) {
    reveal.hide()
    if (!currentEnvId) return
    try {
      const r = await api.copySecret(currentEnvId, s.name)
      toast({
        title:
          r.clearInSeconds > 0
            ? `Copied. Clears in ${r.clearInSeconds} s.`
            : "Copied. This system can't auto-clear the clipboard.",
      })
    } catch (e) {
      if (leftScreen(e)) return
      if (isNotFound(e)) void refresh()
      toast({ title: "Couldn't copy the value. Try again." })
    }
  }

  function openEdit(s: SecretDTO | null) {
    reveal.hide()
    setEditing(s)
    setFormOpen(true)
  }

  function openDelete(s: SecretDTO, trigger: HTMLElement) {
    reveal.hide()
    triggerRef.current = trigger
    deletedRef.current = false
    setDelSecret(s)
    setDelSecretOpen(true)
  }

  // Stable for the memoised rows; reads the latest handlers.
  const handlers = useRef({ copy, openEdit, openDelete, reveal, selName })
  useEffect(() => {
    handlers.current = { copy, openEdit, openDelete, reveal, selName }
  })
  const onAction = useCallback((action: RowAction, s: SecretDTO, trigger: HTMLElement) => {
    const h = handlers.current
    setSelected(s.name)
    if (action === 'reveal') {
      if (h.selName === s.name) void h.reveal.reveal()
      else pendingReveal.current = s.name
    } else if (action === 'copy') void h.copy(s)
    else if (action === 'edit') h.openEdit(s)
    else h.openDelete(s, trigger)
  }, [])
  const onSelect = useCallback((name: string) => setSelected(name), [])

  async function deleteSecret(s: SecretDTO) {
    if (!currentEnvId) return
    try {
      await api.deleteSecret(currentEnvId, s.name)
    } catch (e) {
      if (!isNotFound(e)) throw e // already gone counts as deleted
    }
    deletedRef.current = true
    setSelected(null)
    await refresh()
    toast({ title: 'Secret deleted' })
  }

  async function deleteEnv(e: EnvDTO) {
    try {
      await api.deleteEnvironment(e.id)
    } catch (err) {
      if (!isNotFound(err)) throw err
    }
    deletedRef.current = true
    await loadEnvs() // the selection effect moves to the first remaining env
    toast({ title: 'Environment deleted' })
  }

  const q = query.toLowerCase()
  const shown = secrets?.filter((s) => s.name.toLowerCase().includes(q)) ?? []
  const hasSecrets = secrets !== null && secrets.length > 0

  const newSecretButton = (
    <Button ref={newRef} onClick={() => openEdit(null)}>
      <PlusIcon aria-hidden className="size-4" />
      New secret
    </Button>
  )

  return (
    <div className="flex h-full min-h-0 flex-col">
      <header className="flex h-14 shrink-0 items-center gap-2 border-b border-border px-6">
        <nav aria-label="Breadcrumb">
          <ol className="flex items-center gap-2 text-sm">
            <li>
              <Button
                variant="ghost"
                size="sm"
                className="-ml-3 text-muted-foreground"
                onClick={() => dispatch({ type: 'OPEN_PROJECTS' })}
              >
                Projects
              </Button>
            </li>
            <li aria-hidden className="text-muted-foreground">
              /
            </li>
          </ol>
        </nav>
        <h1 className="truncate text-lg font-semibold tracking-tight text-foreground" title={projectName ?? undefined}>
          {projectName ?? <Skeleton className="h-5 w-32" />}
        </h1>
      </header>

      <div className="flex min-h-0 flex-1">
        <EnvSidebar
          envs={envs}
          currentId={currentEnvId}
          onSelect={(id) => {
            if (id !== currentEnvId) dispatch({ type: 'SELECT_ENV', envId: id })
          }}
          onNew={() => setNewEnvOpen(true)}
          onDelete={(e, trigger) => {
            triggerRef.current = trigger
            deletedRef.current = false
            setDelEnv(e)
            setDelEnvOpen(true)
          }}
          newRef={newEnvRef}
        />

        <section aria-labelledby={env ? 'env-title' : undefined} className="flex min-w-0 flex-1 flex-col">
          {envsFailed && (
            <div className="p-6">
              <ErrorAlert text="Couldn't load this project." onRetry={() => void loadEnvs(true)} />
            </div>
          )}

          {envs !== null && envs.length === 0 && (
            <div className="p-6">
              <Empty
                icon={<FolderIcon aria-hidden />}
                title="No environments yet"
                description="Add an environment, like Development or Production, to keep its secrets."
                action={
                  <Button ref={newEnvRef} onClick={() => setNewEnvOpen(true)}>
                    <PlusIcon aria-hidden className="size-4" />
                    New environment
                  </Button>
                }
              />
            </div>
          )}

          {env && (
            <>
              <div className="flex shrink-0 flex-wrap items-center gap-3 px-6 pb-3 pt-5">
                <h2 id="env-title" className="text-base font-semibold text-foreground">
                  {env.name}
                </h2>
                {hasSecrets && (
                  <span className="text-sm text-muted-foreground tabular-nums">{plural(secrets.length)}</span>
                )}
                {hasSecrets && (
                  <>
                    <div className="relative ml-auto w-56">
                      <SearchIcon
                        aria-hidden
                        className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
                      />
                      <Input
                        ref={searchRef}
                        type="search"
                        aria-label="Search secrets"
                        placeholder="Search secrets"
                        autoComplete="off"
                        spellCheck={false}
                        className="pl-8"
                        value={query}
                        onChange={(e) => setQuery(e.target.value)}
                        onKeyDown={(e) => {
                          if (e.key === 'Escape') setQuery('')
                        }}
                      />
                    </div>
                    {newSecretButton}
                  </>
                )}
              </div>

              <div className="min-h-0 flex-1 overflow-auto px-6 pb-6">
                {secretsFailed && (
                  <ErrorAlert text="Couldn't load secrets." onRetry={() => currentEnvId && void loadSecrets(currentEnvId)} />
                )}
                {secrets === null && !secretsFailed && <TableSkeleton />}
                {secrets !== null && secrets.length === 0 && (
                  <Empty
                    icon={<KeyIcon aria-hidden />}
                    title={`No secrets in ${env.name} yet`}
                    description="Add the first secret of this environment."
                    action={newSecretButton}
                  />
                )}
                {hasSecrets && shown.length === 0 && (
                  <div className="flex flex-col items-center gap-2 py-12 text-center">
                    <p className="text-sm text-muted-foreground">No secrets match</p>
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
                  <SecretTable
                    envName={env.name}
                    secrets={shown}
                    selected={selected}
                    onSelect={onSelect}
                    onAction={onAction}
                    bodyRef={bodyRef}
                  />
                )}
              </div>
            </>
          )}

          {envs === null && !envsFailed && (
            <div className="px-6 pt-5">
              <TableSkeleton />
            </div>
          )}
        </section>

        {hasSecrets && (
          <SecretDetail
            secret={selectedSecret}
            reveal={reveal}
            onCopy={(s) => void copy(s)}
            onEdit={(s) => openEdit(s)}
            onDelete={openDelete}
          />
        )}
      </div>

      {currentEnvId && (
        <SecretForm
          open={formOpen}
          onOpenChange={setFormOpen}
          envId={currentEnvId}
          editing={editing}
          onEnvGone={() => void loadEnvs()}
          onSaved={async (name) => {
            setSelected(name)
            await refresh()
            toast({ title: 'Saved' })
            rescueFocus()
          }}
        />
      )}
      <NewEnv
        open={newEnvOpen}
        onOpenChange={setNewEnvOpen}
        projectId={projectId}
        onCreated={async (e) => {
          await loadEnvs()
          dispatch({ type: 'SELECT_ENV', envId: e.id })
          toast({ title: 'Environment created' })
          setTimeout(rescueFocus) // after the new env's empty state renders
        }}
      />
      <ConfirmDelete
        open={delSecretOpen}
        onOpenChange={setDelSecretOpen}
        title="Delete this secret?"
        description={
          delSecret &&
          env && (
            <>
              Deletes <span className="font-mono font-medium text-foreground">{delSecret.name}</span> from{' '}
              <span className="font-medium text-foreground">{env.name}</span>. This can't be undone.
            </>
          )
        }
        confirmLabel="Delete secret"
        onConfirm={() => (delSecret ? deleteSecret(delSecret) : Promise.resolve())}
        finalFocus={() =>
          deletedRef.current
            ? (bodyRef.current?.querySelector<HTMLElement>('tr[tabindex="0"]') ?? newRef.current)
            : triggerRef.current
        }
      />
      <ConfirmDelete
        open={delEnvOpen}
        onOpenChange={setDelEnvOpen}
        title="Delete this environment?"
        description={
          delEnv && (
            <>
              Deletes <span className="font-medium text-foreground">{delEnv.name}</span>
              {delEnv.secretCount > 0 ? ` and its ${plural(delEnv.secretCount)}` : ''}. This can't be undone.
            </>
          )
        }
        confirmLabel="Delete environment"
        requireText={delEnv && delEnv.secretCount > 0 ? delEnv.name : undefined}
        onConfirm={() => (delEnv ? deleteEnv(delEnv) : Promise.resolve())}
        finalFocus={() => (deletedRef.current ? newEnvRef.current : triggerRef.current)}
      />
    </div>
  )
}

function ErrorAlert({ text, onRetry }: { text: string; onRetry: () => void }) {
  return (
    <Alert variant="destructive" className="flex items-center gap-3">
      <WarningCircleIcon aria-hidden className="size-4" />
      <AlertDescription className="mr-auto">{text}</AlertDescription>
      <Button variant="secondary" size="sm" onClick={onRetry}>
        Try again
      </Button>
    </Alert>
  )
}

function TableSkeleton() {
  return (
    <div aria-busy="true">
      <p role="status" className="sr-only">
        Loading secrets
      </p>
      <div className="flex flex-col gap-2">
        {[0, 1, 2, 3, 4].map((i) => (
          <Skeleton key={i} className="h-9 rounded-md" />
        ))}
      </div>
    </div>
  )
}

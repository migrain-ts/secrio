import { useState } from 'react'
import { ApiError } from '@/api/errors'
import { LockIcon, WarningCircleIcon } from '@/components/icons'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { useApp } from '@/state/AppProvider'

/** The app shell header on data screens: product, vault (switcher when there
 *  are several) and Lock, always one click away (ui notes: security UX). */
export function TopBar() {
  const { api, vault, vaults, screen, dispatch, syncVault } = useApp()
  const [busy, setBusy] = useState(false)
  const [lockFailed, setLockFailed] = useState(false)

  async function lock() {
    setBusy(true)
    setLockFailed(false)
    try {
      await api.lock()
    } catch (e) {
      // locked/no_vault: the session is already closed, same as success.
      // Anything else: the vault may still be open, so never claim it locked.
      const closed = e instanceof ApiError && (e.code === 'locked' || e.code === 'no_vault')
      if (!closed) {
        setBusy(false)
        setLockFailed(true)
        return
      }
    }
    dispatch({ type: 'LOCKED', vaultId: vault?.id ?? null, returnTo: screen })
  }

  async function switchTo(id: string) {
    if (id === vault?.id) return
    try {
      await api.selectVault(id)
      const { current } = await syncVault()
      dispatch(current.unlocked ? { type: 'OPEN_PROJECTS' } : { type: 'GOTO_UNLOCK', vaultId: id })
    } catch {
      // guarded api handled locked/no_vault; stay put otherwise
    }
  }

  return (
    <>
      <header className="flex h-12 shrink-0 items-center gap-3 border-b border-border bg-background px-4">
        <span className="text-sm font-semibold text-foreground">Secrio</span>
        <span aria-hidden className="text-muted-foreground">/</span>
        {vaults.length > 1 ? (
          <Select
            value={vault?.id ?? null}
            onValueChange={(v) => void switchTo(v as string)}
            items={vaults.map((v) => ({ value: v.id, label: v.name }))}
          >
            <SelectTrigger aria-label="Vault" className="h-8 w-auto min-w-36 gap-2">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {vaults.map((v) => (
                <SelectItem key={v.id} value={v.id}>
                  {v.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        ) : (
          <span className="text-sm text-muted-foreground">{vault?.name}</span>
        )}
        {/* Never disabled-looking: a lock in progress keeps the button as is. */}
        <Button variant="secondary" size="sm" className="ml-auto" onClick={() => void lock()} aria-busy={busy}>
          <LockIcon aria-hidden className="size-4" />
          Lock
        </Button>
      </header>
      {lockFailed && (
        <Alert variant="destructive" className="rounded-none border-x-0 border-t-0 py-2.5">
          <WarningCircleIcon aria-hidden className="size-4" />
          <AlertDescription>Couldn't lock the vault. Try again.</AlertDescription>
        </Alert>
      )}
    </>
  )
}

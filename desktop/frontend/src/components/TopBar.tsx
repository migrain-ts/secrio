import { useState } from 'react'
import { LockIcon } from '@/components/icons'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { useApp } from '@/state/AppProvider'

/** The app shell header on data screens: product, vault (switcher when there
 *  are several) and Lock, always one click away (ui notes: security UX). */
export function TopBar() {
  const { api, vault, vaults, screen, dispatch, syncVault } = useApp()
  const [busy, setBusy] = useState(false)

  async function lock() {
    setBusy(true)
    try {
      await api.lock()
    } catch {
      // a locked/no_vault error already moved us to unlock; anything else:
      // go to unlock anyway, the next action re-checks the session
    }
    dispatch({ type: 'LOCKED', vaultId: vault?.id ?? null, returnTo: screen })
    setBusy(false)
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
  )
}

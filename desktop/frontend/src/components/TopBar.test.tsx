import { screen, waitFor } from '@testing-library/react'
import { describe, expect, test } from 'vitest'
import type { Api } from '@/api/client'
import { ApiError } from '@/api/errors'
import { createMockApi } from '@/api/mock'
import { app, renderApp } from '@/test/app'

const onProjects = () => screen.findByRole('heading', { level: 1, name: 'Projects' })

describe('TopBar', () => {
  test('is a banner with the product and vault name, and Lock returns to unlock', async () => {
    const m = createMockApi()
    const { user } = renderApp(m)
    await onProjects()
    const banner = screen.getByRole('banner')
    expect(banner).toHaveTextContent('Secrio')
    expect(banner).toHaveTextContent('work')
    expect(screen.queryByRole('combobox')).toBeNull() // one vault: no switcher
    await user.click(screen.getByRole('button', { name: 'Lock' }))
    expect(await screen.findByRole('heading', { level: 1, name: 'Unlock your vault' })).toBeInTheDocument()
    expect((await m.currentVault()).unlocked).toBe(false)
    expect(screen.queryByRole('banner')).toBeNull()
    expect(screen.queryByRole('status')).toBeNull() // the user locked it: no notice
  })

  test('a failed lock stays on the data screen, says so, and a retry works', async () => {
    const m = createMockApi()
    let broken = true
    const api: Api = {
      ...m,
      lock: () => (broken ? Promise.reject(new ApiError('internal', 'disk error')) : m.lock()),
    }
    const { user } = renderApp(api)
    await onProjects()
    const lockButton = screen.getByRole('button', { name: 'Lock' })
    await user.click(lockButton)
    expect(await screen.findByRole('alert')).toHaveTextContent("Couldn't lock the vault. Try again.")
    expect(screen.queryByText(/disk error/)).toBeNull()
    expect(app.screen).toEqual({ kind: 'projects' })
    expect(screen.getByRole('banner')).toHaveTextContent('work')
    expect(lockButton).toBeEnabled()
    expect((await m.currentVault()).unlocked).toBe(true)
    broken = false
    await user.click(lockButton)
    expect(await screen.findByRole('heading', { level: 1, name: 'Unlock your vault' })).toBeInTheDocument()
    expect((await m.currentVault()).unlocked).toBe(false)
  })

  test.each<['locked' | 'no_vault']>([['locked'], ['no_vault']])(
    'lock rejecting %s (already locked) behaves like success',
    async (code) => {
      const m = createMockApi()
      const api: Api = { ...m, lock: () => Promise.reject(new ApiError(code, 'x')) }
      const { user } = renderApp(api)
      await onProjects()
      await user.click(screen.getByRole('button', { name: 'Lock' }))
      expect(await screen.findByRole('heading', { level: 1, name: 'Unlock your vault' })).toBeInTheDocument()
      expect(screen.queryByText("Couldn't lock the vault. Try again.")).toBeNull()
    },
  )

  test('the switcher appears with several vaults; a locked vault lands on unlock', async () => {
    const m = createMockApi()
    const personal = await m.createVault('personal', 'pw-personal') // selected + unlocked
    await m.lock() // personal locked
    await m.selectVault('v-work') // work is still unlocked
    const { user } = renderApp(m)
    await onProjects()
    const switcher = screen.getByRole('combobox', { name: 'Vault' })
    expect(switcher).toHaveTextContent('work')
    await user.click(switcher)
    await user.click(await screen.findByRole('option', { name: 'personal' }))
    expect(await screen.findByRole('heading', { level: 1, name: 'Unlock your vault' })).toBeInTheDocument()
    expect(app.screen).toEqual({ kind: 'unlock', vaultId: personal.id })
  })

  test('switching to an unlocked vault stays on projects with the new name', async () => {
    const m = createMockApi()
    await m.createVault('personal', 'pw-personal') // selected + unlocked, work too
    const { user } = renderApp(m)
    await onProjects()
    await user.click(screen.getByRole('combobox', { name: 'Vault' }))
    await user.click(await screen.findByRole('option', { name: 'work' }))
    await waitFor(() => expect(app.vault?.name).toBe('work'))
    expect(app.screen).toEqual({ kind: 'projects' })
  })
})

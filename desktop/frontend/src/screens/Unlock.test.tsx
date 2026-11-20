import { act, screen, waitFor } from '@testing-library/react'
import { describe, expect, test } from 'vitest'
import type { Api } from '@/api/client'
import { ApiError } from '@/api/errors'
import { createMockApi, MOCK_PASSWORD } from '@/api/mock'
import { app, passwordInput, renderApp } from '@/test/app'

const onUnlock = () => screen.findByRole('heading', { level: 1, name: 'Unlock your vault' })
const onProjects = () => screen.findByRole('heading', { level: 1, name: 'Projects' })

describe('Unlock', () => {
  test('focus lands on the password field and Enter submits', async () => {
    const { user } = renderApp(createMockApi({ locked: true }))
    await onUnlock()
    expect(passwordInput()).toHaveFocus()
    await user.keyboard(`${MOCK_PASSWORD}{Enter}`)
    await onProjects()
  })

  test('the password field is a plain, unremembered password input', async () => {
    renderApp(createMockApi({ locked: true }))
    await onUnlock()
    const input = passwordInput()
    expect(input).toHaveAttribute('type', 'password')
    expect(input).toHaveAttribute('autocomplete', 'off')
    expect(input).toHaveAttribute('spellcheck', 'false')
    expect(input).toHaveAttribute('autocapitalize', 'off')
  })

  test('a wrong password shows "Wrong password", clears and refocuses the field', async () => {
    const { user } = renderApp(createMockApi({ locked: true }))
    await onUnlock()
    await user.type(passwordInput(), 'nope')
    await user.click(screen.getByRole('button', { name: 'Unlock' }))
    expect(await screen.findByText('Wrong password')).toBeInTheDocument()
    expect(passwordInput()).toHaveValue('')
    expect(passwordInput()).toHaveFocus()
    expect(passwordInput()).toHaveAttribute('aria-invalid', 'true')
    expect(passwordInput()).toHaveAccessibleDescription('Wrong password')
    // the answer arrives after a wait: announced, not only described
    expect(screen.getByRole('alert')).toHaveTextContent('Wrong password')
  })

  test('the password is gone from the DOM as soon as it is submitted, and a loading button keeps its label', async () => {
    const { user } = renderApp(createMockApi({ locked: true, latencyMs: 40 }))
    await onUnlock()
    await user.type(passwordInput(), MOCK_PASSWORD)
    await user.keyboard('{Enter}')
    const button = screen.getByRole('button', { name: 'Unlock' })
    expect(button).toBeDisabled()
    // no way to leave mid-unlock (a late answer would pull the user back)
    expect(screen.getByRole('button', { name: 'Create a new vault' })).toBeDisabled()
    expect(passwordInput()).toHaveValue('')
    expect(document.body.innerHTML).not.toContain(MOCK_PASSWORD)
    await onProjects()
    expect(document.body.innerHTML).not.toContain(MOCK_PASSWORD)
  })

  test('other errors show a short neutral message', async () => {
    const m = createMockApi({ locked: true })
    const api: Api = { ...m, unlock: () => Promise.reject(new ApiError('internal', 'boom')) }
    const { user } = renderApp(api)
    await onUnlock()
    await user.type(passwordInput(), 'x{Enter}')
    expect(await screen.findByRole('alert')).toHaveTextContent("Couldn't unlock the vault. Try again.")
    expect(screen.queryByText('boom')).toBeNull()
    expect(passwordInput()).toHaveValue('')
  })

  test('the vault select appears only with more than one vault', async () => {
    renderApp(createMockApi({ locked: true }))
    await onUnlock()
    expect(screen.queryByRole('combobox')).toBeNull()
    expect(screen.getByText('work')).toBeInTheDocument()
  })

  test('with several vaults, the preselected vault is chosen and listed first', async () => {
    const m = createMockApi()
    await m.createVault('alpha', 'pw-alpha') // selected and unlocked
    await m.lock()
    const { user } = renderApp(m)
    await onUnlock()
    const select = screen.getByRole('combobox', { name: 'Vault' })
    expect(select).toHaveTextContent('alpha')
    await user.click(select)
    const options = await screen.findAllByRole('option')
    expect(options.map((o) => o.textContent)).toEqual(['alpha', 'work'])
    await user.click(screen.getByRole('option', { name: 'work' }))
    await user.click(passwordInput())
    await user.keyboard(`${MOCK_PASSWORD}{Enter}`)
    await onProjects()
    expect(app.vault?.name).toBe('work')
  })

  test('locked during an action: the involuntary notice, then the selection comes back after unlock', async () => {
    const m = createMockApi()
    const { user } = renderApp(m)
    await onProjects()
    act(() => app.dispatch({ type: 'OPEN_PROJECT', projectId: 'p-hotenb', envId: 'e-hotenb-staging' }))
    m.__test.lockedByCli()
    await act(async () => {
      await app.api.listProjects().catch(() => {})
    })
    await onUnlock()
    expect(screen.getByRole('status')).toHaveTextContent(
      'Your vault was locked while you were working. Your last change was not saved.',
    )
    await user.keyboard(`${MOCK_PASSWORD}{Enter}`)
    await waitFor(() =>
      expect(app.screen).toEqual({ kind: 'project', projectId: 'p-hotenb', envId: 'e-hotenb-staging' }),
    )
  })

  test('locked noticed by the focus check: the short notice', async () => {
    const m = createMockApi()
    renderApp(m)
    await onProjects()
    m.__test.lockedByCli()
    act(() => void window.dispatchEvent(new Event('focus')))
    await onUnlock()
    expect(screen.getByRole('status')).toHaveTextContent(/^Your vault was locked\.$/)
  })

  test('"Create a new vault" opens the create form', async () => {
    const { user } = renderApp(createMockApi({ locked: true }))
    await onUnlock()
    await user.click(screen.getByRole('button', { name: 'Create a new vault' }))
    expect(await screen.findByRole('heading', { level: 1, name: 'Create a vault' })).toBeInTheDocument()
  })

  test('keyboard order: password, Unlock, Create a new vault', async () => {
    const { user } = renderApp(createMockApi({ locked: true }))
    await onUnlock()
    expect(passwordInput()).toHaveFocus()
    await user.tab()
    expect(screen.getByRole('button', { name: 'Unlock' })).toHaveFocus()
    await user.tab()
    expect(screen.getByRole('button', { name: 'Create a new vault' })).toHaveFocus()
  })
})

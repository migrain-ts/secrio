import { screen, waitFor } from '@testing-library/react'
import type { UserEvent } from '@testing-library/user-event'
import { describe, expect, test } from 'vitest'
import type { Api } from '@/api/client'
import { ApiError } from '@/api/errors'
import { createMockApi } from '@/api/mock'
import type { ErrorCode } from '@/api/types'
import { app, renderApp } from '@/test/app'

const onCreate = () => screen.findByRole('heading', { level: 1, name: 'Create your first vault' })
const onProjects = () => screen.findByRole('heading', { level: 1, name: 'Projects' })
const nameInput = () => screen.getByLabelText<HTMLInputElement>('Vault name')
const pw = () => screen.getByLabelText<HTMLInputElement>('Password')
const confirm = () => screen.getByLabelText<HTMLInputElement>('Confirm password')
const submit = (user: UserEvent) => user.click(screen.getByRole('button', { name: 'Create vault' }))

async function fill(user: UserEvent, name: string, password: string, again = password) {
  if (name) await user.type(nameInput(), name)
  if (password) await user.type(pw(), password)
  if (again) await user.type(confirm(), again)
}

describe('CreateVault', () => {
  test('first run: title, hint, no way back to unlock, plain inputs', async () => {
    renderApp(createMockApi({ noVault: true }))
    await onCreate()
    expect(nameInput()).toHaveFocus()
    expect(nameInput()).toHaveAccessibleDescription(
      'Letters, digits, dots, dashes and underscores. Up to 64 characters.',
    )
    expect(screen.queryByRole('button', { name: 'Back to unlock' })).toBeNull()
    for (const el of [nameInput(), pw(), confirm()]) expect(el).toHaveAttribute('autocomplete', 'off')
    expect(pw()).toHaveAttribute('type', 'password')
    expect(confirm()).toHaveAttribute('type', 'password')
  })

  test('name errors appear on blur, not while typing', async () => {
    const { user } = renderApp(createMockApi({ noVault: true }))
    await onCreate()
    await user.type(nameInput(), 'con')
    expect(screen.queryByText('This name is reserved by Windows. Choose another.')).toBeNull()
    await user.tab()
    expect(screen.getByText('This name is reserved by Windows. Choose another.')).toBeInTheDocument()
    expect(nameInput()).toHaveAttribute('aria-invalid', 'true')
    expect(nameInput()).toHaveAccessibleDescription('This name is reserved by Windows. Choose another.')
  })

  test.each([
    ['a b', 'Use only letters, digits, dots, dashes and underscores.'],
    ['.hidden', 'Start with a letter, digit or underscore.'],
    ['-x', 'Start with a letter, digit or underscore.'],
    ['a..b', "Don't use two dots in a row."],
    ['LPT9.txt', 'This name is reserved by Windows. Choose another.'],
    ['a'.repeat(65), 'Use 64 characters or fewer.'],
  ])('name %j is refused on submit with %j, and nothing is sent', async (name, msg) => {
    const m = createMockApi({ noVault: true })
    const { user } = renderApp(m)
    await onCreate()
    await fill(user, name, 'pw')
    await submit(user)
    expect(screen.getByText(msg)).toBeInTheDocument()
    expect(nameInput()).toHaveFocus()
    expect(await m.listVaults()).toEqual([])
  })

  test('empty name, empty password and mismatch are refused client-side', async () => {
    const { user } = renderApp(createMockApi({ noVault: true }))
    await onCreate()
    await submit(user)
    expect(screen.getByText('Enter a vault name.')).toBeInTheDocument()
    expect(screen.getByText('Enter a password.')).toBeInTheDocument()
    await fill(user, 'work', 'one', 'two')
    await submit(user)
    expect(screen.getByText("Passwords don't match.")).toBeInTheDocument()
    expect(confirm()).toHaveFocus()
  })

  test.each<[ErrorCode, string, string]>([
    ['duplicate', 'Vault name', 'A vault with this name already exists.'],
    ['invalid_name', 'Vault name', "This name isn't allowed. Choose another."],
    ['invalid_value', 'Password', 'Enter a password.'],
  ])('server %s is shown on the %s field', async (code, field, msg) => {
    const m = createMockApi({ noVault: true })
    const api: Api = { ...m, createVault: () => Promise.reject(new ApiError(code, 'x')) }
    const { user } = renderApp(api)
    await onCreate()
    await fill(user, 'work', 'secret-pw')
    await submit(user)
    const input = screen.getByLabelText(field)
    await waitFor(() => expect(input).toHaveAccessibleDescription(msg))
    expect(input).toHaveFocus()
    expect(pw()).toHaveValue('')
    expect(confirm()).toHaveValue('')
    expect(nameInput()).toHaveValue('work')
  })

  test('a real duplicate (case-insensitive) from the mock lands on the name field', async () => {
    const m = createMockApi({ locked: true }) // has "work"
    const { user } = renderApp(m)
    await screen.findByRole('heading', { level: 1, name: 'Unlock your vault' })
    await user.click(screen.getByRole('button', { name: 'Create a new vault' }))
    await screen.findByRole('heading', { level: 1, name: 'Create a vault' })
    await fill(user, 'WORK', 'pw')
    await submit(user)
    await waitFor(() => expect(nameInput()).toHaveAccessibleDescription('A vault with this name already exists.'))
  })

  test('other server errors show a form-level alert', async () => {
    const m = createMockApi({ noVault: true })
    const api: Api = { ...m, createVault: () => Promise.reject(new ApiError('internal', 'disk full at /x')) }
    const { user } = renderApp(api)
    await onCreate()
    await fill(user, 'work', 'pw')
    await submit(user)
    expect(await screen.findByRole('alert')).toHaveTextContent("Couldn't create the vault. Try again.")
    expect(screen.queryByText(/disk full/)).toBeNull()
  })

  test('success creates, unlocks and opens projects; passwords never stay in the DOM', async () => {
    const m = createMockApi({ noVault: true, latencyMs: 30 })
    const { user } = renderApp(m)
    await onCreate()
    await fill(user, 'personal', 'long-secret-pw')
    await submit(user)
    expect(screen.getByRole('button', { name: 'Create vault' })).toBeDisabled()
    expect(pw()).toHaveValue('')
    expect(confirm()).toHaveValue('')
    expect(document.body.innerHTML).not.toContain('long-secret-pw')
    await onProjects()
    expect(app.vault?.name).toBe('personal')
    expect((await m.currentVault()).unlocked).toBe(true)
  })

  test('back to unlock when a vault exists', async () => {
    const { user } = renderApp(createMockApi({ locked: true }))
    await screen.findByRole('heading', { level: 1, name: 'Unlock your vault' })
    await user.click(screen.getByRole('button', { name: 'Create a new vault' }))
    await user.click(await screen.findByRole('button', { name: 'Back to unlock' }))
    expect(await screen.findByRole('heading', { level: 1, name: 'Unlock your vault' })).toBeInTheDocument()
  })
})

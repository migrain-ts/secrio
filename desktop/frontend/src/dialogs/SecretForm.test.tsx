import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, test, vi } from 'vitest'
import type { Api } from '@/api/client'
import { ApiError } from '@/api/errors'
import { createMockApi, type MockApi } from '@/api/mock'
import type { ErrorCode, SecretDTO } from '@/api/types'
import { SecretForm } from '@/dialogs/SecretForm'
import { AppProvider } from '@/state/AppProvider'

const ENV = 'e-hotenb-development'
const JWT: SecretDTO = {
  name: 'JWT_SECRET',
  type: 'password',
  description: '',
  createdAt: '2026-10-02T09:14:00Z',
  updatedAt: '2026-11-12T16:40:00Z',
}
const STRIPE: SecretDTO = { ...JWT, name: 'STRIPE_KEY', type: 'api_key', description: 'Test mode only' }
/** The names the screen had loaded (a subset of the mock's Development). */
const LOADED = ['API_KEY', 'JWT_SECRET']

function Harness({ editing, onSaved, onEnvGone }: { editing: SecretDTO | null; onSaved: (n: string) => void; onEnvGone: () => void }) {
  const [open, setOpen] = useState(false)
  return (
    <>
      <button type="button" onClick={() => setOpen(true)}>
        Open form
      </button>
      <SecretForm
        open={open}
        onOpenChange={setOpen}
        envId={ENV}
        editing={editing}
        envName="Development"
        existingNames={LOADED}
        onSaved={onSaved}
        onEnvGone={onEnvGone}
      />
    </>
  )
}

async function setup(api: Api = createMockApi(), editing: SecretDTO | null = null) {
  const onSaved = vi.fn()
  const onEnvGone = vi.fn()
  const user = userEvent.setup()
  render(
    <AppProvider api={api} backendStatus="mock">
      <Harness editing={editing} onSaved={onSaved} onEnvGone={onEnvGone} />
    </AppProvider>,
  )
  const open = async () => {
    await user.click(screen.getByRole('button', { name: 'Open form' }))
    return screen.findByRole('dialog', { name: editing ? 'Edit secret' : 'New secret' })
  }
  const dialog = await open()
  return { user, onSaved, onEnvGone, dialog, open }
}

const nameInput = () => screen.getByLabelText<HTMLInputElement>('Name')
const valueInput = () => screen.getByLabelText<HTMLTextAreaElement>('Value')
const typeSelect = () => screen.getByRole('combobox', { name: 'Type' })
const save = () => screen.getByRole('button', { name: 'Save' })

function failingSave(code: ErrorCode, m: MockApi = createMockApi()) {
  const saveSecret = vi.fn(() => Promise.reject(new ApiError(code, 'server text s3cr3t')))
  return { api: { ...m, saveSecret } as Api, saveSecret }
}

describe('SecretForm: create', () => {
  test('fields: name, masked monospace value with a Show toggle, type defaulting to Text', async () => {
    const { user } = await setup()
    await waitFor(() => expect(nameInput()).toHaveFocus())
    const v = valueInput()
    expect(v.tagName).toBe('TEXTAREA')
    for (const [attr, val] of [
      ['autocomplete', 'off'],
      ['spellcheck', 'false'],
      ['autocapitalize', 'off'],
      ['autocorrect', 'off'],
    ]) {
      expect(v).toHaveAttribute(attr, val)
    }
    expect(v.className).toContain('[-webkit-text-security:disc]')
    expect(v.className).toContain('font-mono')
    expect(typeSelect()).toHaveTextContent('Text')
    const toggle = screen.getByRole('button', { name: 'Show value' })
    expect(toggle).toHaveAttribute('aria-pressed', 'false')
    await user.click(toggle)
    expect(toggle).toHaveAttribute('aria-pressed', 'true')
    expect(valueInput().className).not.toContain('[-webkit-text-security:disc]')
  })

  test('saves name, multi-line value and chosen type', async () => {
    const m = createMockApi()
    const spy = vi.spyOn(m, 'saveSecret')
    const { user, onSaved } = await setup(m)
    await user.type(nameInput(), 'NEW_KEY')
    await user.type(valueInput(), 'line1{Enter}line2')
    await user.click(typeSelect())
    await user.click(await screen.findByRole('option', { name: 'Password' }))
    await user.click(save())
    await waitFor(() => expect(onSaved).toHaveBeenCalledWith('NEW_KEY'))
    expect(spy).toHaveBeenCalledWith(ENV, 'NEW_KEY', 'line1\nline2', 'password')
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(await m.revealSecret(ENV, 'NEW_KEY')).toBe('line1\nline2')
  })

  test('the eight types have human labels', async () => {
    const { user } = await setup()
    await user.click(typeSelect())
    const options = await screen.findAllByRole('option')
    expect(options.map((o) => o.textContent)).toEqual([
      'Text',
      'Password',
      'API key',
      'Token',
      'URL',
      'Private key',
      'Certificate',
      'JSON',
    ])
  })

  test.each([
    ['1ABC', 'starts with a digit'],
    ['MY-KEY', 'dash'],
    ['MY KEY', 'space'],
    ['', 'empty'],
    ['CLÉ', 'non-ASCII'],
  ])('name %j (%s) is refused before any call', async (name) => {
    const m = createMockApi()
    const spy = vi.spyOn(m, 'saveSecret')
    const { user } = await setup(m)
    if (name) await user.type(nameInput(), name)
    await user.type(valueInput(), 'v')
    await user.click(save())
    await waitFor(() =>
      expect(nameInput()).toHaveAccessibleDescription("Use letters, digits and underscores; don't start with a digit"),
    )
    expect(nameInput()).toHaveAttribute('aria-invalid', 'true')
    expect(nameInput()).toHaveFocus()
    expect(spy).not.toHaveBeenCalled()
  })

  test('name error shows on blur', async () => {
    const { user } = await setup()
    await user.type(nameInput(), '9X')
    await user.tab()
    expect(nameInput()).toHaveAccessibleDescription("Use letters, digits and underscores; don't start with a digit")
  })

  test('an empty value is refused', async () => {
    const m = createMockApi()
    const spy = vi.spyOn(m, 'saveSecret')
    const { user } = await setup(m)
    await user.type(nameInput(), 'OK_NAME')
    await user.click(save())
    await waitFor(() => expect(valueInput()).toHaveAccessibleDescription('A value is required'))
    expect(valueInput()).toHaveFocus()
    expect(spy).not.toHaveBeenCalled()
  })

  test('a value over 1 MiB (UTF-8 bytes) is refused; exactly 1 MiB is accepted', async () => {
    const m = createMockApi()
    const spy = vi.spyOn(m, 'saveSecret')
    const { user, onSaved } = await setup(m)
    await user.type(nameInput(), 'BIG')
    // 'é' is 2 bytes: 524289 of them is 1 MiB + 2 bytes
    fireEvent.change(valueInput(), { target: { value: 'é'.repeat(524_289) } })
    await user.click(save())
    await waitFor(() => expect(valueInput()).toHaveAccessibleDescription('Value is too large (max 1 MiB)'))
    expect(spy).not.toHaveBeenCalled()
    fireEvent.change(valueInput(), { target: { value: 'é'.repeat(524_288) } })
    await user.click(save())
    await waitFor(() => expect(onSaved).toHaveBeenCalledWith('BIG'))
  })

  test('a NUL character cannot get into the value', async () => {
    const m = createMockApi()
    const spy = vi.spyOn(m, 'saveSecret')
    const { user, onSaved } = await setup(m)
    await user.type(nameInput(), 'WITH_NUL')
    fireEvent.change(valueInput(), { target: { value: 'a\0b' } })
    expect(valueInput()).toHaveValue('ab')
    await user.click(save())
    await waitFor(() => expect(onSaved).toHaveBeenCalled())
    expect(spy).toHaveBeenCalledWith(ENV, 'WITH_NUL', 'ab', 'text')
  })

  test.each<[ErrorCode, 'name' | 'value', string]>([
    ['invalid_name', 'name', "Use letters, digits and underscores; don't start with a digit"],
    ['invalid_value', 'value', 'This value is not accepted'],
  ])('server %s maps to the %s field, input preserved, never the server text', async (code, field, copy) => {
    const { api } = failingSave(code)
    const { user, onSaved } = await setup(api)
    await user.type(nameInput(), 'GOOD')
    await user.type(valueInput(), 'kept-value')
    await user.click(save())
    const input = field === 'name' ? nameInput() : valueInput()
    await waitFor(() => expect(input).toHaveAccessibleDescription(copy))
    expect(nameInput()).toHaveValue('GOOD')
    expect(valueInput()).toHaveValue('kept-value')
    expect(onSaved).not.toHaveBeenCalled()
    expect(document.body.innerHTML).not.toContain('server text')
  })

  test('not_found: the environment is gone, the parent refreshes', async () => {
    const { api } = failingSave('not_found')
    const { user, onEnvGone, dialog } = await setup(api)
    await user.type(nameInput(), 'GOOD')
    await user.type(valueInput(), 'v')
    await user.click(save())
    expect(await within(dialog).findByRole('alert')).toHaveTextContent('This environment no longer exists')
    expect(onEnvGone).toHaveBeenCalledOnce()
  })

  test('other errors: a generic form alert', async () => {
    const { api } = failingSave('internal')
    const { user, dialog } = await setup(api)
    await user.type(nameInput(), 'GOOD')
    await user.type(valueInput(), 'v')
    await user.click(save())
    expect(await within(dialog).findByRole('alert')).toHaveTextContent("Couldn't save the secret. Try again.")
    expect(document.body.innerHTML).not.toContain('server text')
  })

  test('the typed value is gone after Cancel, Escape and a successful save', async () => {
    const { user, open, onSaved } = await setup()
    await user.type(valueInput(), 'typed-1')
    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    await open()
    expect(valueInput()).toHaveValue('')
    await user.type(valueInput(), 'typed-2')
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    await open()
    expect(valueInput()).toHaveValue('')
    await user.type(nameInput(), 'SAVED_ONE')
    await user.type(valueInput(), 'typed-3')
    await user.click(save())
    await waitFor(() => expect(onSaved).toHaveBeenCalled())
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    await open()
    expect(nameInput()).toHaveValue('')
    expect(valueInput()).toHaveValue('')
  })

  test('focus returns to the trigger after Cancel', async () => {
    const { user } = await setup()
    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.getByRole('button', { name: 'Open form' })).toHaveFocus())
  })
})

describe('SecretForm: create never overwrites', () => {
  const DUP = 'A secret with this name already exists in Development'

  test('an existing name is refused on blur and on submit, nothing saved, value kept, name focused', async () => {
    const m = createMockApi()
    const spy = vi.spyOn(m, 'saveSecret')
    const { user, onSaved } = await setup(m)
    await user.type(nameInput(), 'JWT_SECRET')
    await user.tab()
    expect(nameInput()).toHaveAccessibleDescription(DUP)
    await user.type(valueInput(), 'would-overwrite')
    await user.click(save())
    await waitFor(() => expect(nameInput()).toHaveFocus())
    expect(nameInput()).toHaveAccessibleDescription(DUP)
    expect(nameInput()).toHaveAttribute('aria-invalid', 'true')
    expect(valueInput()).toHaveValue('would-overwrite')
    expect(spy).not.toHaveBeenCalled()
    expect(onSaved).not.toHaveBeenCalled()
    expect(await m.revealSecret(ENV, 'JWT_SECRET')).toBe('fake-jwt-signing-secret-0000')
  })

  test('a name that differs only by case is another name', async () => {
    const m = createMockApi()
    const spy = vi.spyOn(m, 'saveSecret')
    const { user, onSaved } = await setup(m)
    await user.type(nameInput(), 'jwt_secret')
    await user.type(valueInput(), 'v')
    await user.click(save())
    await waitFor(() => expect(onSaved).toHaveBeenCalledWith('jwt_secret'))
    expect(spy).toHaveBeenCalledWith(ENV, 'jwt_secret', 'v', 'text')
  })

  test('a name created elsewhere since the list was loaded is caught at submit', async () => {
    const m = createMockApi()
    const spy = vi.spyOn(m, 'saveSecret')
    const { user, onSaved } = await setup(m)
    await user.type(nameInput(), 'FROM_CLI')
    await user.type(valueInput(), 'mine')
    await m.saveSecret(ENV, 'FROM_CLI', 'theirs', 'text') // e.g. `secrio set` meanwhile
    spy.mockClear()
    await user.click(save())
    await waitFor(() => expect(nameInput()).toHaveAccessibleDescription(DUP))
    expect(nameInput()).toHaveFocus()
    expect(valueInput()).toHaveValue('mine')
    expect(spy).not.toHaveBeenCalled()
    expect(onSaved).not.toHaveBeenCalled()
    expect(await m.revealSecret(ENV, 'FROM_CLI')).toBe('theirs')
  })

  test('the fresh list fails: a generic alert and nothing saved', async () => {
    const m = createMockApi()
    const saveSecret = vi.spyOn(m, 'saveSecret')
    const api: Api = { ...m, listSecrets: () => Promise.reject(new ApiError('internal', 'server text')) }
    const { user, dialog, onSaved } = await setup(api)
    await user.type(nameInput(), 'NEW_ONE')
    await user.type(valueInput(), 'v')
    await user.click(save())
    expect(await within(dialog).findByRole('alert')).toHaveTextContent("Couldn't save the secret. Try again.")
    expect(saveSecret).not.toHaveBeenCalled()
    expect(onSaved).not.toHaveBeenCalled()
    expect(valueInput()).toHaveValue('v')
    expect(document.body.innerHTML).not.toContain('server text')
  })

  test('edit keeps updating the same name', async () => {
    const m = createMockApi()
    const list = vi.spyOn(m, 'listSecrets')
    const spy = vi.spyOn(m, 'saveSecret')
    const { user, onSaved } = await setup(m, JWT)
    await waitFor(() => expect(valueInput()).toBeEnabled())
    await user.click(save())
    await waitFor(() => expect(onSaved).toHaveBeenCalledWith('JWT_SECRET'))
    expect(spy).toHaveBeenCalledWith(ENV, 'JWT_SECRET', 'fake-jwt-signing-secret-0000', 'password')
    expect(list).not.toHaveBeenCalled()
  })
})

describe('SecretForm: edit', () => {
  test('loads the current value into the masked field (disabled while loading); name fixed', async () => {
    let resolve!: (v: string) => void
    const m = createMockApi()
    const api: Api = { ...m, revealSecret: vi.fn(() => new Promise<string>((r) => (resolve = r))) }
    await setup(api, JWT)
    expect(nameInput()).toBeDisabled()
    expect(nameInput()).toHaveValue('JWT_SECRET')
    expect(valueInput()).toBeDisabled()
    expect(screen.getByText('Loading the current value')).toBeInTheDocument()
    expect(api.revealSecret).toHaveBeenCalledWith(ENV, 'JWT_SECRET')
    resolve('fake-jwt-signing-secret-0000')
    await waitFor(() => expect(valueInput()).toBeEnabled())
    expect(valueInput()).toHaveValue('fake-jwt-signing-secret-0000')
    expect(valueInput().className).toContain('[-webkit-text-security:disc]')
    expect(typeSelect()).toHaveTextContent('Password')
  })

  test('saves a type change with the value; the description is kept', async () => {
    const m = createMockApi()
    const spy = vi.spyOn(m, 'saveSecret')
    const { user, onSaved } = await setup(m, STRIPE)
    await waitFor(() => expect(valueInput()).toBeEnabled())
    await user.click(typeSelect())
    await user.click(await screen.findByRole('option', { name: 'Token' }))
    await user.click(save())
    await waitFor(() => expect(onSaved).toHaveBeenCalledWith('STRIPE_KEY'))
    expect(spy).toHaveBeenCalledWith(ENV, 'STRIPE_KEY', 'fake-stripe-test-key-3333', 'token')
    const s = (await m.listSecrets(ENV)).find((x) => x.name === 'STRIPE_KEY')
    expect(s).toMatchObject({ type: 'token', description: 'Test mode only' })
  })

  test('a failed load shows an alert and disables Save', async () => {
    const m = createMockApi()
    const api: Api = { ...m, revealSecret: () => Promise.reject(new ApiError('internal', 'server text')) }
    const { dialog } = await setup(api, JWT)
    expect(await within(dialog).findByRole('alert')).toHaveTextContent("Couldn't load the current value.")
    expect(save()).toBeDisabled()
    expect(document.body.innerHTML).not.toContain('server text')
  })

  test('the loaded value is dropped on close', async () => {
    const { user, open } = await setup(createMockApi(), JWT)
    await waitFor(() => expect(valueInput()).toHaveValue('fake-jwt-signing-secret-0000'))
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(document.body.innerHTML).not.toContain('fake-jwt-signing-secret-0000')
    await open()
    await waitFor(() => expect(valueInput()).toHaveValue('fake-jwt-signing-secret-0000'))
  })
})

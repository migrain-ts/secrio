import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, test, vi } from 'vitest'
import type { Api } from '@/api/client'
import { ApiError } from '@/api/errors'
import { createMockApi } from '@/api/mock'
import type { ErrorCode, ProjectDTO } from '@/api/types'
import { NewProject } from '@/dialogs/NewProject'
import { AppProvider } from '@/state/AppProvider'

function Harness({ onCreated }: { onCreated: (p: ProjectDTO) => void }) {
  const [open, setOpen] = useState(false)
  return (
    <>
      <button type="button" onClick={() => setOpen(true)}>
        New project
      </button>
      <NewProject open={open} onOpenChange={setOpen} onCreated={onCreated} />
    </>
  )
}

async function setup(api: Api = createMockApi()) {
  const onCreated = vi.fn()
  const user = userEvent.setup()
  render(
    <AppProvider api={api} backendStatus="mock">
      <Harness onCreated={onCreated} />
    </AppProvider>,
  )
  await user.click(screen.getByRole('button', { name: 'New project' }))
  const dialog = await screen.findByRole('dialog', { name: 'New project' })
  const input = screen.getByLabelText<HTMLInputElement>('Name')
  return { user, onCreated, dialog, input }
}

const failing = (code: ErrorCode) => {
  const m = createMockApi()
  const createProject = vi.fn((_name: string) => Promise.reject(new ApiError(code, 'x')))
  return { api: { ...m, createProject } as Api, createProject }
}

describe('NewProject', () => {
  test('the name field has focus, is not remembered, and Enter creates', async () => {
    const { user, onCreated, input } = await setup()
    await waitFor(() => expect(input).toHaveFocus())
    expect(input).toHaveAttribute('autocomplete', 'off')
    await user.keyboard('billing{Enter}')
    await waitFor(() => expect(onCreated).toHaveBeenCalledWith(expect.objectContaining({ name: 'billing' })))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(screen.getByRole('button', { name: 'New project' })).toHaveFocus()
  })

  test('the Create button submits too', async () => {
    const { user, onCreated, input } = await setup()
    await user.type(input, 'billing')
    await user.click(screen.getByRole('button', { name: 'Create' }))
    await waitFor(() => expect(onCreated).toHaveBeenCalledTimes(1))
  })

  test('the name is sent exactly as typed, spaces included', async () => {
    const m = createMockApi()
    const spy = vi.spyOn(m, 'createProject')
    const { user, input } = await setup(m)
    await user.type(input, '  billing  {Enter}')
    await waitFor(() => expect(spy).toHaveBeenCalledWith('  billing  '))
  })

  test('an empty name cannot be submitted', async () => {
    const m = createMockApi()
    const spy = vi.spyOn(m, 'createProject')
    const { user, input } = await setup(m)
    await user.click(screen.getByRole('button', { name: 'Create' }))
    expect(spy).not.toHaveBeenCalled()
    expect(input).toHaveAccessibleDescription('Enter a project name')
    expect(input).toHaveAttribute('aria-invalid', 'true')
    expect(screen.getByRole('dialog')).toBeInTheDocument()
  })

  test.each([
    ['duplicate', 'A project with this name already exists'],
    ['invalid_name', 'Use a name without control characters'],
  ] as const)('%s is shown under the field, the dialog stays open with the input', async (code, text) => {
    const { api } = failing(code)
    const { user, onCreated, input } = await setup(api)
    await user.type(input, 'hotenb{Enter}')
    await waitFor(() => expect(input).toHaveAccessibleDescription(text))
    expect(input).toHaveAttribute('aria-invalid', 'true')
    expect(input).toHaveValue('hotenb')
    expect(input).toHaveFocus()
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(onCreated).not.toHaveBeenCalled()
  })

  test('other errors show a generic form alert and keep the input', async () => {
    const { api } = failing('internal')
    const { user, input } = await setup(api)
    await user.type(input, 'billing{Enter}')
    expect(await screen.findByRole('alert')).toHaveTextContent("Couldn't create the project. Try again.")
    expect(input).toHaveValue('billing')
    expect(input).not.toHaveAttribute('aria-invalid')
    expect(screen.getByRole('dialog')).toBeInTheDocument()
  })

  test('Cancel and Escape close without calling the api and restore focus', async () => {
    const m = createMockApi()
    const spy = vi.spyOn(m, 'createProject')
    const { user, input } = await setup(m)
    await user.type(input, 'billing')
    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(screen.getByRole('button', { name: 'New project' })).toHaveFocus()
    await user.click(screen.getByRole('button', { name: 'New project' }))
    const again = await screen.findByLabelText('Name')
    expect(again).toHaveValue('') // a new dialog starts empty
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(screen.getByRole('button', { name: 'New project' })).toHaveFocus()
    expect(spy).not.toHaveBeenCalled()
  })

  test('busy: a double Enter creates once', async () => {
    const m = createMockApi({ latencyMs: 30 })
    const spy = vi.spyOn(m, 'createProject')
    const { user, input } = await setup(m)
    await user.type(input, 'billing')
    await user.keyboard('{Enter}{Enter}')
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(spy).toHaveBeenCalledTimes(1)
  })
})

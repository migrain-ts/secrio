import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, test, vi } from 'vitest'
import type { Api } from '@/api/client'
import { ApiError } from '@/api/errors'
import { createMockApi } from '@/api/mock'
import type { EnvDTO, ErrorCode } from '@/api/types'
import { NewEnv } from '@/dialogs/NewEnv'
import { AppProvider } from '@/state/AppProvider'

function Harness({ onCreated }: { onCreated: (e: EnvDTO) => void }) {
  const [open, setOpen] = useState(false)
  return (
    <>
      <button type="button" onClick={() => setOpen(true)}>
        New environment
      </button>
      <NewEnv open={open} onOpenChange={setOpen} projectId="p-hotenb" onCreated={onCreated} />
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
  await user.click(screen.getByRole('button', { name: 'New environment' }))
  await screen.findByRole('dialog', { name: 'New environment' })
  const input = screen.getByLabelText<HTMLInputElement>('Name')
  return { user, onCreated, input }
}

const failing = (code: ErrorCode) => {
  const m = createMockApi()
  return { ...m, createEnvironment: vi.fn(() => Promise.reject(new ApiError(code, 'x'))) } as Api
}

describe('NewEnv', () => {
  test('focus in the name field; Enter creates in the project and focus returns', async () => {
    const m = createMockApi()
    const spy = vi.spyOn(m, 'createEnvironment')
    const { user, onCreated, input } = await setup(m)
    await waitFor(() => expect(input).toHaveFocus())
    expect(input).toHaveAttribute('autocomplete', 'off')
    await user.keyboard('QA{Enter}')
    await waitFor(() => expect(onCreated).toHaveBeenCalledWith(expect.objectContaining({ name: 'QA', secretCount: 0 })))
    expect(spy).toHaveBeenCalledWith('p-hotenb', 'QA')
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(screen.getByRole('button', { name: 'New environment' })).toHaveFocus()
  })

  test('an empty name cannot be submitted', async () => {
    const m = createMockApi()
    const spy = vi.spyOn(m, 'createEnvironment')
    const { user, input } = await setup(m)
    await user.click(screen.getByRole('button', { name: 'Create' }))
    expect(spy).not.toHaveBeenCalled()
    expect(input).toHaveAccessibleDescription('Enter an environment name')
  })

  test.each([
    ['duplicate', 'An environment with this name already exists'],
    ['invalid_name', 'Use a name without control characters'],
  ] as const)('%s is shown under the field with the input kept', async (code, text) => {
    const { user, onCreated, input } = await setup(failing(code))
    await user.type(input, 'Staging{Enter}')
    await waitFor(() => expect(input).toHaveAccessibleDescription(text))
    expect(input).toHaveAttribute('aria-invalid', 'true')
    expect(input).toHaveValue('Staging')
    expect(onCreated).not.toHaveBeenCalled()
  })

  test('the real duplicate rule of the mock', async () => {
    const { user, input } = await setup()
    await user.type(input, 'Staging{Enter}')
    await waitFor(() => expect(input).toHaveAccessibleDescription('An environment with this name already exists'))
  })

  test('other errors: a generic alert', async () => {
    const { user, input } = await setup(failing('internal'))
    await user.type(input, 'QA{Enter}')
    expect(await screen.findByRole('alert')).toHaveTextContent("Couldn't create the environment. Try again.")
  })
})

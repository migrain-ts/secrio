import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, test, vi } from 'vitest'
import { ConfirmDelete } from '@/dialogs/ConfirmDelete'

/** Opens the dialog from a real trigger so focus restore can be checked. */
function Harness({ onConfirm, requireText }: { onConfirm: () => Promise<void>; requireText?: string }) {
  const [open, setOpen] = useState(false)
  return (
    <>
      <button type="button" onClick={() => setOpen(true)}>
        Open
      </button>
      <ConfirmDelete
        open={open}
        onOpenChange={setOpen}
        title="Delete this project?"
        description="Deletes hotenb, its 3 environments and 12 secrets. This can't be undone."
        confirmLabel="Delete project"
        requireText={requireText}
        onConfirm={onConfirm}
      />
    </>
  )
}

async function setup(props: { onConfirm?: () => Promise<void>; requireText?: string } = {}) {
  const onConfirm = vi.fn(props.onConfirm ?? (() => Promise.resolve()))
  const user = userEvent.setup()
  render(<Harness onConfirm={onConfirm} requireText={props.requireText} />)
  await user.click(screen.getByRole('button', { name: 'Open' }))
  const dialog = await screen.findByRole('alertdialog', { name: 'Delete this project?' })
  const confirm = () => screen.getByRole('button', { name: 'Delete project' })
  return { user, onConfirm, dialog, confirm }
}

describe('ConfirmDelete', () => {
  test('without requireText, Cancel has the focus so Enter does not destroy', async () => {
    const { user, onConfirm, dialog } = await setup()
    expect(dialog).toHaveAccessibleDescription(/can't be undone/)
    await waitFor(() => expect(screen.getByRole('button', { name: 'Cancel' })).toHaveFocus())
    await user.keyboard('{Enter}')
    expect(onConfirm).not.toHaveBeenCalled()
    await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
    expect(screen.getByRole('button', { name: 'Open' })).toHaveFocus()
  })

  test('the confirm button is destructive and confirms without requireText', async () => {
    const { user, onConfirm, confirm } = await setup()
    expect(confirm()).toBeEnabled()
    await user.click(confirm())
    expect(onConfirm).toHaveBeenCalledTimes(1)
    await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
  })

  test('with requireText the input has focus and confirm waits for the exact text', async () => {
    const { user, onConfirm, confirm } = await setup({ requireText: 'hotenb' })
    const input = screen.getByLabelText('Type hotenb to confirm')
    await waitFor(() => expect(input).toHaveFocus())
    expect(input).toHaveAttribute('autocomplete', 'off')
    expect(confirm()).toBeDisabled()
    await user.type(input, 'Hotenb') // case differs
    expect(confirm()).toBeDisabled()
    await user.keyboard('{Enter}') // disabled: Enter does nothing
    expect(onConfirm).not.toHaveBeenCalled()
    await user.clear(input)
    await user.type(input, 'hotenb ') // near miss: trailing space
    expect(confirm()).toBeDisabled()
    await user.keyboard('{Backspace}')
    expect(confirm()).toBeEnabled()
    await user.keyboard('{Enter}')
    expect(onConfirm).toHaveBeenCalledTimes(1)
  })

  test('a rejecting onConfirm keeps the dialog open with an error, and retry works', async () => {
    let fail = true
    const { user, onConfirm, confirm } = await setup({
      onConfirm: () => (fail ? Promise.reject(new Error('boom')) : Promise.resolve()),
    })
    await user.click(confirm())
    expect(await screen.findByRole('alert')).toHaveTextContent("Couldn't delete. Try again.")
    expect(screen.getByRole('alertdialog')).toBeInTheDocument()
    expect(document.body.textContent).not.toContain('boom')
    fail = false
    await user.click(confirm())
    expect(onConfirm).toHaveBeenCalledTimes(2)
    await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
  })

  test('Escape cancels without confirming and restores focus', async () => {
    const { user, onConfirm } = await setup({ requireText: 'hotenb' })
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
    expect(onConfirm).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: 'Open' })).toHaveFocus()
  })

  test('busy: a double click confirms once and the button is disabled meanwhile', async () => {
    let resolve!: () => void
    const { user, onConfirm, confirm } = await setup({ onConfirm: () => new Promise<void>((r) => (resolve = r)) })
    await user.dblClick(confirm())
    expect(onConfirm).toHaveBeenCalledTimes(1)
    expect(confirm()).toBeDisabled()
    resolve()
    await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
  })

  test('reopening starts fresh: no typed text, no old error', async () => {
    const { user } = await setup({ requireText: 'hotenb', onConfirm: () => Promise.reject(new Error('x')) })
    await user.type(screen.getByLabelText('Type hotenb to confirm'), 'hotenb{Enter}')
    await screen.findByRole('alert')
    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
    await user.click(screen.getByRole('button', { name: 'Open' }))
    await screen.findByRole('alertdialog')
    expect(screen.getByLabelText('Type hotenb to confirm')).toHaveValue('')
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })
})

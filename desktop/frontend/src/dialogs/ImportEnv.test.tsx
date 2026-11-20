import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, test, vi } from 'vitest'
import type { Api } from '@/api/client'
import { ApiError } from '@/api/errors'
import { createMockApi, MOCK_IMPORT_PATH, type MockApi } from '@/api/mock'
import type { ErrorCode, ImportPreview } from '@/api/types'
import { ImportEnv } from '@/dialogs/ImportEnv'
import type { ImportSummary } from '@/dialogs/importMachine'
import { AppProvider } from '@/state/AppProvider'

const ENV = 'e-hotenb-staging' // has SENTRY_DSN and LOG_LEVEL
const VALUE = 's3cr3t-VALUE'
const FILE = { NEW_ONE: VALUE, SENTRY_DSN: VALUE, NEW_TWO: VALUE }

function mockWithFile(entries: Record<string, string> | null = FILE): MockApi {
  const m = createMockApi()
  m.__test.setFileContent(MOCK_IMPORT_PATH, entries)
  return m
}

function Harness({ onImported }: { onImported: (s: ImportSummary) => void }) {
  const [open, setOpen] = useState(false)
  return (
    <>
      <button type="button" onClick={() => setOpen(true)}>
        Open import
      </button>
      <ImportEnv open={open} onOpenChange={setOpen} envId={ENV} envName="Staging" onImported={onImported} />
    </>
  )
}

async function setup(api: Api = mockWithFile()) {
  const onImported = vi.fn()
  const user = userEvent.setup()
  render(
    <AppProvider api={api} backendStatus="mock">
      <Harness onImported={onImported} />
    </AppProvider>,
  )
  await user.click(screen.getByRole('button', { name: 'Open import' }))
  const dialog = await screen.findByRole('dialog', { name: 'Import .env file' })
  return { user, onImported, dialog }
}

const noValue = () => expect(document.body.innerHTML).not.toContain(VALUE)
const btn = (d: HTMLElement, name: string | RegExp) => within(d).getByRole('button', { name })
const list = (d: HTMLElement, name: string) => within(d).getByRole('list', { name })
const items = (d: HTMLElement, name: string) => within(list(d, name)).getAllByRole('listitem').map((li) => li.textContent)

async function toPreview(user: ReturnType<typeof userEvent.setup>, d: HTMLElement) {
  await user.click(btn(d, 'Choose file'))
  await waitFor(() => expect(primaryOf(d).textContent).toMatch(/^Import \d|^Close without importing$/))
}

/** The last footer button: the primary action of the step. */
function primaryOf(d: HTMLElement): HTMLElement {
  const buttons = within(d).getAllByRole('button').filter((b) => !b.querySelector('.sr-only'))
  return buttons[buttons.length - 1]
}

/** A promise the test resolves by hand. */
function deferred<T>() {
  let resolve!: (v: T) => void
  let reject!: (e: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

describe('ImportEnv: idle', () => {
  test('named and described, the primary "Choose file" has focus', async () => {
    const { dialog } = await setup()
    expect(dialog).toHaveAccessibleDescription(
      'Nothing changes until you confirm.',
    )
    await waitFor(() => expect(btn(dialog, 'Choose file')).toHaveFocus())
  })

  test('a cancelled picker leaves idle with no error and no toast', async () => {
    const m = mockWithFile()
    m.__test.setPickResult('')
    const preview = vi.spyOn(m, 'previewImport')
    const { user, dialog } = await setup(m)
    await user.click(btn(dialog, 'Choose file'))
    await waitFor(() => expect(btn(dialog, 'Choose file')).toBeEnabled())
    expect(within(dialog).queryByRole('alert')).toBeNull()
    expect(screen.queryByRole('alert')).toBeNull()
    expect(preview).not.toHaveBeenCalled()
    expect(btn(dialog, 'Choose file')).toHaveFocus()
  })
})

describe('ImportEnv: preview', () => {
  test('counts and name lists, names only, never a value', async () => {
    const { user, dialog } = await setup()
    noValue()
    await toPreview(user, dialog)
    expect(within(dialog).getByText('2 new, 1 already exists')).toBeInTheDocument()
    expect(items(dialog, 'Will be added')).toEqual(['NEW_ONE', 'NEW_TWO'])
    expect(items(dialog, 'Already in Staging')).toEqual(['SENTRY_DSN'])
    noValue()
    await waitFor(() => expect(btn(dialog, 'Import 2 secrets')).toHaveFocus())
  })

  test('the strategy: skip by default, replace shows the warning and changes the count', async () => {
    const { user, dialog } = await setup()
    await toPreview(user, dialog)
    const group = within(dialog).getByRole('radiogroup', { name: 'Existing secrets' })
    const skip = within(group).getByRole('radio', { name: 'Skip' })
    const replace = within(group).getByRole('radio', { name: 'Replace' })
    expect(skip).toBeChecked()
    expect(replace).not.toBeChecked()
    expect(within(dialog).queryByRole('alert')).toBeNull()
    await user.click(replace)
    expect(replace).toBeChecked()
    expect(within(dialog).getByRole('alert')).toHaveTextContent(
      'Replaces 1 value permanently.',
    )
    expect(btn(dialog, 'Import 3 secrets')).toBeEnabled()
    await user.click(skip)
    expect(within(dialog).queryByRole('alert')).toBeNull()
    expect(btn(dialog, 'Import 2 secrets')).toBeInTheDocument()
  })

  test('no strategy choice when nothing already exists', async () => {
    const { user, dialog } = await setup(mockWithFile({ A_NEW: VALUE }))
    await toPreview(user, dialog)
    expect(within(dialog).getByText('1 new')).toBeInTheDocument()
    expect(within(dialog).queryByRole('radiogroup')).toBeNull()
    expect(within(dialog).queryByRole('list', { name: 'Already in Staging' })).toBeNull()
    expect(btn(dialog, 'Import 1 secret')).toBeInTheDocument()
  })

  test('nothing new to import: "Close without importing" closes', async () => {
    const m = mockWithFile({ SENTRY_DSN: VALUE, LOG_LEVEL: VALUE })
    const apply = vi.spyOn(m, 'applyImport')
    const { user, dialog, onImported } = await setup(m)
    await toPreview(user, dialog)
    expect(within(dialog).getByText('No new names, 2 already exist')).toBeInTheDocument()
    expect(within(dialog).queryByText('Nothing new to import.')).toBeNull() // said once, by the counts
    await user.click(within(dialog).getByRole('radio', { name: 'Replace' }))
    expect(within(dialog).getByRole('alert')).toHaveTextContent('Replaces 2 values permanently.')
    expect(btn(dialog, 'Import 2 secrets')).toBeInTheDocument()
    await user.click(within(dialog).getByRole('radio', { name: 'Skip' }))
    // one button named "Close" (the corner X); the footer names its outcome
    expect(within(dialog).getAllByRole('button', { name: 'Close' })).toHaveLength(1)
    await user.click(btn(dialog, 'Close without importing'))
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(apply).not.toHaveBeenCalled()
    expect(onImported).not.toHaveBeenCalled()
  })

  test('lists show at most 50 names, then "and K more"', async () => {
    const entries: Record<string, string> = {}
    for (let i = 0; i < 53; i++) entries[`NAME_${String(i).padStart(2, '0')}`] = VALUE
    const { user, dialog } = await setup(mockWithFile(entries))
    await toPreview(user, dialog)
    expect(items(dialog, 'Will be added')).toHaveLength(50)
    expect(within(dialog).getByText('and 3 more')).toBeInTheDocument()
    expect(btn(dialog, 'Import 53 secrets')).toBeInTheDocument()
  })

  test('names are text, never HTML', async () => {
    const pv: ImportPreview = { new: ['<b>X</b>'], duplicate: [] }
    const m = mockWithFile()
    const { user, dialog } = await setup({ ...m, previewImport: () => Promise.resolve(pv) })
    await toPreview(user, dialog)
    expect(items(dialog, 'Will be added')).toEqual(['<b>X</b>'])
    expect(dialog.querySelector('b')).toBeNull()
  })

  test('"Choose another file" picks again and shows the new file', async () => {
    const m = mockWithFile()
    m.__test.setFileContent('/other/.env', { OTHER_NAME: VALUE })
    const { user, dialog } = await setup(m)
    await toPreview(user, dialog)
    await user.click(within(dialog).getByRole('radio', { name: 'Replace' }))
    m.__test.setPickResult('/other/.env')
    await user.click(btn(dialog, 'Choose another file'))
    expect(await within(dialog).findByText('1 new')).toBeInTheDocument()
    expect(items(dialog, 'Will be added')).toEqual(['OTHER_NAME'])
    expect(within(dialog).queryByRole('list', { name: 'Already in Staging' })).toBeNull()
  })
})

describe('ImportEnv: apply', () => {
  test('skip: imports, closes and reports the counts', async () => {
    const m = mockWithFile()
    const apply = vi.spyOn(m, 'applyImport')
    const { user, dialog, onImported } = await setup(m)
    await toPreview(user, dialog)
    await user.click(btn(dialog, 'Import 2 secrets'))
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(apply).toHaveBeenCalledWith(ENV, MOCK_IMPORT_PATH, 'skip')
    expect(onImported).toHaveBeenCalledWith({ imported: 2, replaced: 0, skipped: 1 })
    noValue()
  })

  test('replace: overwrite strategy and the replaced count', async () => {
    const m = mockWithFile()
    const apply = vi.spyOn(m, 'applyImport')
    const { user, dialog, onImported } = await setup(m)
    await toPreview(user, dialog)
    await user.click(within(dialog).getByRole('radio', { name: 'Replace' }))
    await user.click(btn(dialog, 'Import 3 secrets'))
    await waitFor(() => expect(onImported).toHaveBeenCalledWith({ imported: 2, replaced: 1, skipped: 0 }))
    expect(apply).toHaveBeenCalledWith(ENV, MOCK_IMPORT_PATH, 'overwrite')
  })

  test('while applying: busy, controls disabled, Escape blocked, a double submit sends once', async () => {
    const m = mockWithFile()
    const gate = deferred<void>()
    const apply = vi.fn(() => gate.promise)
    const { user, dialog, onImported } = await setup({ ...m, applyImport: apply })
    await toPreview(user, dialog)
    const primary = btn(dialog, 'Import 2 secrets')
    fireEvent.click(primary)
    fireEvent.click(primary)
    expect(apply).toHaveBeenCalledTimes(1)
    await waitFor(() => expect(primary).toBeDisabled())
    expect(within(dialog).getByRole('status')).toHaveTextContent('Importing')
    expect(within(dialog).getByRole('radio', { name: 'Replace' })).toHaveAttribute('aria-disabled', 'true')
    expect(btn(dialog, 'Choose another file')).toBeDisabled()
    expect(btn(dialog, 'Cancel')).toBeDisabled()
    expect(btn(dialog, 'Close')).toBeDisabled() // the corner X too
    await user.keyboard('{Escape}')
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    await act(async () => gate.resolve())
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(onImported).toHaveBeenCalledTimes(1)
  })

  test('conflict: the message, a fresh preview, nothing imported until confirmed again', async () => {
    const m = mockWithFile()
    const apply = vi.spyOn(m, 'applyImport')
    const { user, dialog, onImported } = await setup(m)
    await toPreview(user, dialog)
    await user.click(within(dialog).getByRole('radio', { name: 'Replace' }))
    // the vault changed: NEW_ONE now exists
    await m.saveSecret(ENV, 'NEW_ONE', 'other', 'text')
    await user.click(btn(dialog, 'Import 3 secrets'))
    expect(await within(dialog).findByText('1 new, 2 already exist')).toBeInTheDocument()
    expect(within(dialog).getByText('The file or the vault changed. Review the new preview.')).toBeInTheDocument()
    expect(items(dialog, 'Already in Staging')).toEqual(['NEW_ONE', 'SENTRY_DSN'])
    expect(onImported).not.toHaveBeenCalled()
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    // back to the safe choice, focused (Enter is not one key away from an overwrite); nothing written
    const skip = within(dialog).getByRole('radio', { name: 'Skip' })
    expect(skip).toBeChecked()
    await waitFor(() => expect(skip).toHaveFocus())
    expect(within(dialog).queryByText(/^Replaces /)).toBeNull()
    expect(btn(dialog, 'Import 1 secret')).toBeInTheDocument()
    expect((await m.listSecrets(ENV)).map((s) => s.name)).not.toContain('NEW_TWO')
    expect(apply).toHaveBeenCalledTimes(1)
    // an informed choice again
    await user.click(within(dialog).getByRole('radio', { name: 'Replace' }))
    expect(within(dialog).getByText(/Replaces 2 values/)).toBeInTheDocument()
    await user.click(btn(dialog, 'Import 3 secrets'))
    await waitFor(() => expect(onImported).toHaveBeenCalledWith({ imported: 1, replaced: 2, skipped: 0 }))
    expect((await m.listSecrets(ENV)).map((s) => s.name)).toContain('NEW_TWO')
  })

  test('a conflict with no duplicates left: focus on the primary', async () => {
    const m = mockWithFile({ NEW_ONE: VALUE })
    const { user, dialog } = await setup(m)
    await toPreview(user, dialog)
    m.__test.setFileContent(MOCK_IMPORT_PATH, { NEW_ONE: VALUE, NEW_TWO: VALUE }) // the file changed
    await user.click(btn(dialog, 'Import 1 secret'))
    expect(await within(dialog).findByText('2 new')).toBeInTheDocument()
    expect(within(dialog).getByText('The file or the vault changed. Review the new preview.')).toBeInTheDocument()
    await waitFor(() => expect(btn(dialog, 'Import 2 secrets')).toHaveFocus())
  })

  test('the file vanished before apply: its own error, the dialog stays usable', async () => {
    const m = mockWithFile()
    const { user, dialog } = await setup(m)
    await toPreview(user, dialog)
    m.__test.setFileContent(MOCK_IMPORT_PATH, null) // the file is gone now
    await user.click(btn(dialog, 'Import 2 secrets'))
    expect(await within(dialog).findByText('That file could not be found.')).toBeInTheDocument()
    expect(btn(dialog, 'Choose another file')).toBeEnabled()
  })

  test('conflict, then the automatic re-preview fails: fixed copy, usable, nothing applied', async () => {
    const m = mockWithFile()
    m.__test.setFileContent('/other/.env', { OTHER_NAME: VALUE })
    let previews = 0
    const apply = vi.fn(() => Promise.reject(new ApiError('conflict', `server says ${VALUE}`)))
    const { user, dialog, onImported } = await setup({
      ...m,
      applyImport: apply,
      previewImport: (env, path) =>
        ++previews === 2 ? Promise.reject(new ApiError('invalid_value', 'server text')) : m.previewImport(env, path),
    })
    await toPreview(user, dialog)
    await user.click(within(dialog).getByRole('radio', { name: 'Replace' }))
    await user.click(btn(dialog, 'Import 3 secrets'))
    expect(await within(dialog).findByRole('alert')).toHaveTextContent(
      "This file has an entry Secrio can't import, or it is too large.",
    )
    expect(dialog).not.toHaveTextContent('server')
    expect(previews).toBe(2)
    expect(apply).toHaveBeenCalledTimes(1)
    expect(onImported).not.toHaveBeenCalled()
    expect((await m.listSecrets(ENV)).map((s) => s.name)).not.toContain('NEW_ONE')
    noValue()
    await waitFor(() => expect(btn(dialog, 'Try again')).toHaveFocus())
    m.__test.setPickResult('/other/.env')
    await user.click(btn(dialog, 'Choose another file'))
    expect(await within(dialog).findByText('1 new')).toBeInTheDocument()
  })
})

describe('ImportEnv: errors', () => {
  test.each<[string, Record<string, string> | null, string]>([
    ['an unparsable file', { 'bad name': VALUE }, "This file has an entry Secrio can't import, or it is too large."],
    ['a missing file', null, 'That file could not be found.'],
  ])('%s: fixed copy, the dialog stays usable', async (_, entries, text) => {
    const m = mockWithFile(entries)
    const { user, dialog } = await setup(m)
    await user.click(btn(dialog, 'Choose file'))
    expect(await within(dialog).findByRole('alert')).toHaveTextContent(text)
    noValue()
    await waitFor(() => expect(btn(dialog, 'Try again')).toHaveFocus())
    // fix the file, try again
    m.__test.setFileContent(MOCK_IMPORT_PATH, FILE)
    await user.click(btn(dialog, 'Try again'))
    expect(await within(dialog).findByText('2 new, 1 already exists')).toBeInTheDocument()
  })

  test.each<ErrorCode>(['unsupported', 'internal', 'duplicate'])('%s on apply: generic copy, never the server text', async (code) => {
    const m = mockWithFile()
    const { user, dialog } = await setup({
      ...m,
      applyImport: () => Promise.reject(new ApiError(code, `server says ${VALUE}`)),
    })
    await toPreview(user, dialog)
    await user.click(within(dialog).getByRole('radio', { name: 'Replace' }))
    await user.click(btn(dialog, 'Import 3 secrets'))
    expect(await within(dialog).findByRole('alert')).toHaveTextContent('Import failed. Try again.')
    expect(dialog).not.toHaveTextContent('server says')
    noValue()
    // the choice survives "Try again"
    await user.click(btn(dialog, 'Try again'))
    await within(dialog).findByText('2 new, 1 already exists')
    expect(within(dialog).getByRole('radio', { name: 'Replace' })).toBeChecked()
  })

  test('a failing picker: generic copy and "Choose file" again', async () => {
    const m = mockWithFile()
    const { user, dialog } = await setup({ ...m, pickEnvFile: () => Promise.reject(new ApiError('unsupported', 'x')) })
    await user.click(btn(dialog, 'Choose file'))
    expect(await within(dialog).findByRole('alert')).toHaveTextContent('Import failed. Try again.')
    expect(btn(dialog, 'Choose file')).toBeEnabled()
  })

  test('"Choose another file" from an error', async () => {
    const m = mockWithFile(null)
    m.__test.setFileContent('/other/.env', { OTHER_NAME: VALUE })
    const { user, dialog } = await setup(m)
    await user.click(btn(dialog, 'Choose file'))
    await within(dialog).findByRole('alert')
    m.__test.setPickResult('/other/.env')
    await user.click(btn(dialog, 'Choose another file'))
    expect(await within(dialog).findByText('1 new')).toBeInTheDocument()
    expect(within(dialog).queryByRole('alert')).toBeNull()
  })
})

describe('ImportEnv: stale results', () => {
  test('a late preview of the first file is ignored after choosing another', async () => {
    const m = mockWithFile()
    m.__test.setFileContent('/other/.env', { OTHER_NAME: VALUE })
    const first = deferred<ImportPreview>()
    const preview = vi.fn((env: string, path: string) =>
      path === MOCK_IMPORT_PATH ? first.promise : m.previewImport(env, path),
    )
    const { user, dialog } = await setup({ ...m, previewImport: preview })
    await user.click(btn(dialog, 'Choose file'))
    await waitFor(() => expect(preview).toHaveBeenCalledTimes(1))
    expect(within(dialog).getByRole('status')).toHaveTextContent('Reading the file')
    m.__test.setPickResult('/other/.env')
    await user.click(btn(dialog, 'Choose another file'))
    expect(await within(dialog).findByText('1 new')).toBeInTheDocument()
    await act(async () => first.resolve({ new: ['STALE_NAME'], duplicate: [] }))
    expect(within(dialog).getByText('1 new')).toBeInTheDocument()
    expect(within(dialog).queryByText('STALE_NAME')).toBeNull()
  })

  test('a late preview after closing is ignored; reopening starts at the picker', async () => {
    const m = mockWithFile()
    const first = deferred<ImportPreview>()
    const { user, dialog } = await setup({ ...m, previewImport: () => first.promise })
    await user.click(btn(dialog, 'Choose file'))
    await user.click(btn(dialog, 'Cancel'))
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    await act(async () => first.resolve({ new: ['STALE_NAME'], duplicate: [] }))
    await user.click(screen.getByRole('button', { name: 'Open import' }))
    const again = await screen.findByRole('dialog', { name: 'Import .env file' })
    expect(btn(again, 'Choose file')).toBeEnabled()
    expect(within(again).queryByText('STALE_NAME')).toBeNull()
  })
})

describe('ImportEnv: keyboard', () => {
  test('the whole flow with the keyboard only', async () => {
    const m = mockWithFile()
    const { user, dialog, onImported } = await setup(m)
    await waitFor(() => expect(btn(dialog, 'Choose file')).toHaveFocus())
    await user.keyboard('{Enter}')
    await waitFor(() => expect(btn(dialog, 'Import 2 secrets')).toHaveFocus())
    // reach the radios with Shift+Tab, move with the arrows
    const skip = within(dialog).getByRole('radio', { name: 'Skip' })
    for (let i = 0; i < 10 && document.activeElement !== skip; i++) await user.tab({ shift: true })
    expect(skip).toHaveFocus()
    await user.keyboard('{ArrowDown}')
    expect(within(dialog).getByRole('radio', { name: 'Replace' })).toBeChecked()
    expect(within(dialog).getByRole('alert')).toBeInTheDocument()
    // back to the primary with Tab, then Space
    const primary = btn(dialog, 'Import 3 secrets')
    for (let i = 0; i < 10 && document.activeElement !== primary; i++) await user.tab()
    expect(primary).toHaveFocus()
    await user.keyboard(' ')
    await waitFor(() => expect(onImported).toHaveBeenCalledWith({ imported: 2, replaced: 1, skipped: 0 }))
  })
})

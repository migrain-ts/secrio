import { act, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import type { Api } from '@/api/client'
import { ApiError } from '@/api/errors'
import { createMockApi, MOCK_IMPORT_PATH, MOCK_PASSWORD, type MockApi, type MockOptions } from '@/api/mock'
import type { SecretDTO } from '@/api/types'
import { REVEAL_MS } from '@/hooks/useReveal'
import { app, renderApp } from '@/test/app'

const DEV = 'e-hotenb-development'
const STAGING = 'e-hotenb-staging'
const VALUE = 's3cr3t-VALUE'

/** The mock with a recognisable value in hotenb / Development. */
async function seeded(opts: MockOptions = {}): Promise<MockApi> {
  const m = createMockApi(opts)
  await m.saveSecret(DEV, 'SEEDED', VALUE, 'password')
  return m
}

/** Renders the app and opens hotenb (Development is selected first). */
async function openHotenb(api: Api) {
  const r = renderApp(api)
  await r.user.click(await screen.findByRole('button', { name: 'hotenb' }))
  await table('Development')
  return r
}

const table = (env: string) => screen.findByRole('table', { name: `Secrets in ${env}` })
const rows = () => within(screen.getByRole('table')).getAllByRole('row').slice(1) // minus the header
const rowNames = () => rows().map((r) => within(r).getAllByRole('cell')[0].textContent)
const row = (name: string) => {
  const r = rows().find((x) => within(x).getAllByRole('cell')[0].textContent === name)
  if (!r) throw new Error(`no row ${name}`)
  return r
}
const detail = () => screen.getByRole('complementary', { name: 'Secret details' })
const revealed = () => document.querySelector('[data-revealed]')
const noValueInPage = () => expect(document.body.innerHTML).not.toContain(VALUE)
const nav = () => screen.getByRole('navigation', { name: 'Environments' })
const envButton = (name: string) => within(nav()).getByRole('button', { name: new RegExp(`^${name}`) })

afterEach(() => {
  vi.useRealTimers()
})

describe('Project: layout', () => {
  test('breadcrumb, one h1, the env sidebar with counts, the table and the detail hint', async () => {
    const { user } = await openHotenb(await seeded())
    expect(screen.getAllByRole('heading', { level: 1 })).toHaveLength(1)
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('hotenb')
    const crumbs = screen.getByRole('navigation', { name: 'Breadcrumb' })
    expect(within(crumbs).getByRole('button', { name: 'Projects' })).toBeInTheDocument()
    expect(envButton('Development')).toHaveAccessibleName('Development, 8 secrets')
    expect(envButton('Development')).toHaveAttribute('aria-current', 'page')
    expect(envButton('Staging')).toHaveAccessibleName('Staging, 2 secrets')
    expect(envButton('Staging')).not.toHaveAttribute('aria-current')
    expect(envButton('Production')).toHaveAccessibleName('Production, 6 secrets')
    expect(rowNames()).toEqual([
      'API_KEY',
      'DATABASE_URL',
      'GITHUB_TOKEN',
      'JWT_SECRET',
      'REDIS_URL',
      'SEEDED',
      'SENTRY_DSN',
      'STRIPE_KEY',
    ])
    expect(within(detail()).getByText('Select a secret to see its details.')).toBeInTheDocument()
    await user.click(within(crumbs).getByRole('button', { name: 'Projects' }))
    expect(await screen.findByRole('heading', { level: 1, name: 'Projects' })).toBeInTheDocument()
  })

  test('a row: name, fixed dots + "Hidden", a type badge with text, a short date with the full time as title', async () => {
    await openHotenb(await seeded())
    const cells = within(row('JWT_SECRET')).getAllByRole('cell')
    expect(cells[1]).toHaveTextContent(/^••••••••••Hidden$/)
    expect(cells[1].querySelector('[aria-hidden="true"]')).toHaveTextContent(/^••••••••••$/)
    // the same dots for a short and a long value: never derived from the length
    expect(within(row('API_KEY')).getAllByRole('cell')[1].textContent).toBe(cells[1].textContent)
    expect(cells[2]).toHaveTextContent('Password')
    expect(within(row('DATABASE_URL')).getAllByRole('cell')[2]).toHaveTextContent('URL')
    const time = cells[3].querySelector('time')
    expect(time).toHaveAttribute('datetime', '2026-11-12T16:40:00Z')
    expect(time).toHaveAttribute('title', '2026-11-12T16:40:00Z')
    expect(time).toHaveTextContent('Nov 12, 2026')
    for (const label of ['Reveal', 'Copy', 'Edit', 'Delete']) {
      expect(within(row('JWT_SECRET')).getByRole('button', { name: `${label} JWT_SECRET` })).toBeInTheDocument()
    }
  })

  test('the table has headers with scope and a name; all buttons are named', async () => {
    await openHotenb(await seeded())
    const headers = within(screen.getByRole('table')).getAllByRole('columnheader')
    expect(headers.map((h) => h.textContent)).toEqual(['Name', 'Value', 'Type', 'Updated', 'Actions'])
    for (const h of headers) expect(h).toHaveAttribute('scope', 'col')
    for (const b of screen.getAllByRole('button')) expect(b).toHaveAccessibleName()
    expect(screen.getByRole('searchbox', { name: 'Search secrets' })).toBeInTheDocument()
  })
})

describe('Project: no value in the page', () => {
  test('absent after render, selection, copy, failed reveal and an error; present only once revealed', async () => {
    const m = await seeded()
    let failReveal = true
    const api: Api = {
      ...m,
      revealSecret: (e, n) => (failReveal ? Promise.reject(new ApiError('internal', `boom ${VALUE}`)) : m.revealSecret(e, n)),
    }
    const { user } = await openHotenb(api)
    noValueInPage()
    await user.click(row('SEEDED'))
    expect(row('SEEDED')).toHaveAttribute('aria-current', 'true')
    noValueInPage()
    await user.click(within(detail()).getByRole('button', { name: 'Copy' }))
    await screen.findByText('Copied. Clears in 30 s.')
    noValueInPage()
    await user.click(within(detail()).getByRole('button', { name: 'Reveal' }))
    expect(await within(detail()).findByText("Couldn't show the value.")).toBeInTheDocument()
    noValueInPage()
    failReveal = false
    await user.click(within(detail()).getByRole('button', { name: 'Reveal' }))
    await waitFor(() => expect(revealed()).toHaveTextContent(VALUE))
    // only inside the revealed block
    const html = document.body.innerHTML
    expect(html.split(VALUE)).toHaveLength(2)
    expect(revealed()!.closest('table')).toBeNull()
    expect(screen.getByRole('table').innerHTML).not.toContain(VALUE)
    expect(document.title).not.toContain(VALUE)
  })
})

describe('Project: reveal', () => {
  async function revealSeeded(m?: Api) {
    const r = await openHotenb(m ?? (await seeded()))
    await r.user.click(row('SEEDED'))
    await r.user.click(within(detail()).getByRole('button', { name: 'Reveal' }))
    await waitFor(() => expect(revealed()).toHaveTextContent(VALUE))
    return r
  }

  test(`hides after ${REVEAL_MS / 1000} s`, async () => {
    const { user } = await openHotenb(await seeded())
    await user.click(row('SEEDED'))
    // only timeouts: the kit's popups also use animation frames
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    fireEvent.click(within(detail()).getByRole('button', { name: 'Reveal' }))
    await act(async () => {})
    expect(revealed()).toHaveTextContent(VALUE)
    expect(within(detail()).getByRole('button', { name: 'Hide' })).toBeInTheDocument()
    act(() => vi.advanceTimersByTime(REVEAL_MS - 1))
    expect(revealed()).toHaveTextContent(VALUE)
    act(() => vi.advanceTimersByTime(1))
    expect(revealed()).toBeNull()
    noValueInPage()
    // let any other timeout queued meanwhile (focus, toasts) run before going real
    act(() => vi.runOnlyPendingTimers())
  })

  test('the row Reveal button selects and reveals', async () => {
    const { user } = await openHotenb(await seeded())
    await user.click(within(row('SEEDED')).getByRole('button', { name: 'Reveal SEEDED' }))
    await waitFor(() => expect(revealed()).toHaveTextContent(VALUE))
    expect(row('SEEDED')).toHaveAttribute('aria-current', 'true')
  })

  test('the row Reveal button toggles: Hide NAME while revealed', async () => {
    const { user } = await openHotenb(await seeded())
    await user.click(within(row('SEEDED')).getByRole('button', { name: 'Reveal SEEDED' }))
    await waitFor(() => expect(revealed()).toHaveTextContent(VALUE))
    const hide = within(row('SEEDED')).getByRole('button', { name: 'Hide SEEDED' })
    expect(within(row('API_KEY')).getByRole('button', { name: 'Reveal API_KEY' })).toBeInTheDocument()
    await user.click(hide)
    expect(revealed()).toBeNull()
    expect(within(row('SEEDED')).getByRole('button', { name: 'Reveal SEEDED' })).toBeInTheDocument()
  })

  test('row action buttons carry no native title tooltip', async () => {
    await openHotenb(await seeded())
    for (const b of within(row('SEEDED')).getAllByRole('button')) expect(b).not.toHaveAttribute('title')
  })

  test('Hide hides', async () => {
    const { user } = await revealSeeded()
    await user.click(within(detail()).getByRole('button', { name: 'Hide' }))
    expect(revealed()).toBeNull()
  })

  test('hides on selection change', async () => {
    const { user } = await revealSeeded()
    await user.click(row('API_KEY'))
    expect(revealed()).toBeNull()
    noValueInPage()
  })

  test('hides on env change and clears the selection', async () => {
    const { user } = await revealSeeded()
    await user.click(envButton('Staging'))
    await table('Staging')
    noValueInPage()
    expect(within(detail()).getByText('Select a secret to see its details.')).toBeInTheDocument()
    expect(app.screen).toEqual({ kind: 'project', projectId: 'p-hotenb', envId: STAGING })
  })

  test('hides on window blur', async () => {
    await revealSeeded()
    act(() => void window.dispatchEvent(new Event('blur')))
    noValueInPage()
  })

  test('hides when the page becomes hidden', async () => {
    await revealSeeded()
    const vis = vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden')
    act(() => void document.dispatchEvent(new Event('visibilitychange')))
    noValueInPage()
    vis.mockRestore()
  })

  test('hides on lock, and is not back after unlocking', async () => {
    const { user } = await revealSeeded()
    await user.click(screen.getByRole('button', { name: 'Lock' }))
    await screen.findByRole('heading', { level: 1, name: 'Unlock your vault' })
    noValueInPage()
    await user.keyboard(`${MOCK_PASSWORD}{Enter}`)
    await table('Development')
    noValueInPage()
  })

  test.each(['Copy', 'Edit', 'Delete'])('hides when %s starts', async (action) => {
    const { user } = await revealSeeded()
    await user.click(within(detail()).getByRole('button', { name: action }))
    expect(revealed()).toBeNull()
  })

  test('a reveal of a secret deleted elsewhere refreshes the list', async () => {
    const m = await seeded()
    const { user } = await openHotenb(m)
    await user.click(row('SEEDED'))
    await m.deleteSecret(DEV, 'SEEDED')
    await user.click(within(detail()).getByRole('button', { name: 'Reveal' }))
    await waitFor(() => expect(rowNames()).not.toContain('SEEDED'))
  })
})

describe('Project: copy', () => {
  test('toast with the clear delay; the clipboard is Go-side only', async () => {
    const clip = { writeText: vi.fn(), readText: vi.fn() }
    Object.defineProperty(navigator, 'clipboard', { value: clip, configurable: true })
    const m = await seeded()
    const { user } = await openHotenb(m)
    await user.click(within(row('SEEDED')).getByRole('button', { name: 'Copy SEEDED' }))
    const t = await screen.findByText('Copied. Clears in 30 s.')
    expect(t.closest('[aria-live="polite"]')).not.toBeNull()
    expect(m.__test.clipboardText()).toBe(VALUE)
    expect(clip.writeText).not.toHaveBeenCalled()
    expect(clip.readText).not.toHaveBeenCalled()
    noValueInPage()
  })

  test('no auto-clear on this system', async () => {
    const { user } = await openHotenb(await seeded({ clipboardReadable: false }))
    await user.click(within(row('SEEDED')).getByRole('button', { name: 'Copy SEEDED' }))
    expect(await screen.findByText("Copied. This system can't auto-clear the clipboard.")).toBeInTheDocument()
  })

  test('a failure says so without the server text', async () => {
    const m = await seeded()
    const api: Api = { ...m, copySecret: () => Promise.reject(new ApiError('internal', 'server detail')) }
    const { user } = await openHotenb(api)
    await user.click(within(row('SEEDED')).getByRole('button', { name: 'Copy SEEDED' }))
    expect(await screen.findByText("Couldn't copy the value. Try again.")).toBeInTheDocument()
    expect(document.body.innerHTML).not.toContain('server detail')
  })
})

describe('Project: create and edit', () => {
  test('create: the row appears, is selected, a toast says Saved, focus returns, counts update', async () => {
    const { user } = await openHotenb(await seeded())
    const newButton = screen.getByRole('button', { name: 'New secret' })
    await user.click(newButton)
    await screen.findByRole('dialog', { name: 'New secret' })
    await user.type(screen.getByLabelText('Name'), 'NEW_ONE')
    await user.type(screen.getByLabelText('Value'), 'typed-value')
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'New secret' })).not.toBeInTheDocument())
    await waitFor(() => expect(rowNames()).toContain('NEW_ONE'))
    expect(row('NEW_ONE')).toHaveAttribute('aria-current', 'true')
    expect(await screen.findByText('Saved')).toBeInTheDocument()
    await waitFor(() => expect(newButton).toHaveFocus())
    await waitFor(() => expect(envButton('Development')).toHaveAccessibleName('Development, 9 secrets'))
    expect(document.body.innerHTML).not.toContain('typed-value')
    // reopening starts empty
    await user.click(newButton)
    await screen.findByRole('dialog', { name: 'New secret' })
    expect(screen.getByLabelText('Value')).toHaveValue('')
  })

  test('create refuses a name already in the env (the screen passes its names)', async () => {
    const m = await seeded()
    const spy = vi.spyOn(m, 'saveSecret')
    const { user } = await openHotenb(m)
    await user.click(screen.getByRole('button', { name: 'New secret' }))
    await screen.findByRole('dialog', { name: 'New secret' })
    await user.type(screen.getByLabelText('Name'), 'SEEDED')
    await user.tab()
    expect(screen.getByLabelText('Name')).toHaveAccessibleDescription(
      'A secret with this name already exists in Development',
    )
    await user.type(screen.getByLabelText('Value'), 'other')
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(screen.getByLabelText('Name')).toHaveFocus())
    expect(spy).not.toHaveBeenCalled()
    expect(await m.revealSecret(DEV, 'SEEDED')).toBe(VALUE)
  })

  test('create from the empty state: focus lands on the toolbar New secret', async () => {
    const m = createMockApi()
    await m.createEnvironment('p-hotenb', 'QA')
    const { user } = await openHotenb(m)
    await user.click(envButton('QA'))
    await screen.findByText('No secrets in QA yet')
    await user.click(screen.getByRole('button', { name: 'New secret' }))
    await screen.findByRole('dialog', { name: 'New secret' })
    await user.type(screen.getByLabelText('Name'), 'FIRST')
    await user.type(screen.getByLabelText('Value'), 'v')
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await table('QA')
    await waitFor(() => expect(screen.getByRole('button', { name: 'New secret' })).toHaveFocus())
  })

  test('edit: the current value is loaded masked, the name is fixed, the type change is saved', async () => {
    const m = await seeded()
    const spy = vi.spyOn(m, 'saveSecret')
    const { user } = await openHotenb(m)
    await user.click(within(row('SEEDED')).getByRole('button', { name: 'Edit SEEDED' }))
    await screen.findByRole('dialog', { name: 'Edit secret' })
    expect(screen.getByLabelText('Name')).toBeDisabled()
    await waitFor(() => expect(screen.getByLabelText('Value')).toHaveValue(VALUE))
    await user.click(screen.getByRole('combobox', { name: 'Type' }))
    await user.click(await screen.findByRole('option', { name: 'Token' }))
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Edit secret' })).not.toBeInTheDocument())
    expect(spy).toHaveBeenLastCalledWith(DEV, 'SEEDED', VALUE, 'token')
    await waitFor(() => expect(within(row('SEEDED')).getAllByRole('cell')[2]).toHaveTextContent('Token'))
    noValueInPage()
  })

  test('locked mid-save: unlock with the notice; after unlocking the typed input is not restored', async () => {
    const m = await seeded()
    const { user } = await openHotenb(m)
    await user.click(screen.getByRole('button', { name: 'New secret' }))
    await screen.findByRole('dialog', { name: 'New secret' })
    await user.type(screen.getByLabelText('Name'), 'LOST_ONE')
    await user.type(screen.getByLabelText('Value'), 'typed-and-lost')
    m.__test.lockedByCli()
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await screen.findByRole('heading', { level: 1, name: 'Unlock your vault' })
    expect(screen.getByRole('status')).toHaveTextContent(
      'Your vault was locked while you were working. Your last change was not saved.',
    )
    expect(document.body.innerHTML).not.toContain('typed-and-lost')
    await user.keyboard(`${MOCK_PASSWORD}{Enter}`)
    await table('Development')
    expect(screen.queryByRole('dialog', { name: 'New secret' })).not.toBeInTheDocument()
    expect(rowNames()).not.toContain('LOST_ONE')
    await user.click(screen.getByRole('button', { name: 'New secret' }))
    await screen.findByRole('dialog', { name: 'New secret' })
    expect(screen.getByLabelText('Name')).toHaveValue('')
    expect(screen.getByLabelText('Value')).toHaveValue('')
  })
})

describe('Project: delete', () => {
  test('secret: Cancel has focus; confirm removes the row, clears the selection, focus to the table', async () => {
    const { user } = await openHotenb(await seeded())
    await user.click(row('JWT_SECRET'))
    await user.click(within(detail()).getByRole('button', { name: 'Delete' }))
    const dialog = await screen.findByRole('alertdialog', { name: 'Delete this secret?' })
    expect(dialog).toHaveTextContent("Deletes JWT_SECRET from Development. This can't be undone.")
    await waitFor(() => expect(within(dialog).getByRole('button', { name: 'Cancel' })).toHaveFocus())
    expect(within(dialog).queryByRole('textbox')).toBeNull()
    await user.click(within(dialog).getByRole('button', { name: 'Delete secret' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
    expect(rowNames()).not.toContain('JWT_SECRET')
    expect(within(detail()).getByText('Select a secret to see its details.')).toBeInTheDocument()
    await waitFor(() => expect(rows()[0]).toHaveFocus())
    await waitFor(() => expect(envButton('Development')).toHaveAccessibleName('Development, 7 secrets'))
  })

  test('deleting the last secret: empty state, focus on its New secret button', async () => {
    const m = createMockApi()
    const { user } = renderApp(m)
    await user.click(await screen.findByRole('button', { name: 'hotenb' }))
    await user.click(await within(await screen.findByRole('navigation', { name: 'Environments' })).findByRole('button', { name: /^Staging/ }))
    await table('Staging')
    await m.deleteSecret(STAGING, 'LOG_LEVEL') // gone behind the screen's back
    await user.click(within(row('SENTRY_DSN')).getByRole('button', { name: 'Delete SENTRY_DSN' }))
    await user.click(await screen.findByRole('button', { name: 'Delete secret' }))
    expect(await screen.findByText('No secrets in Staging yet')).toBeInTheDocument()
    await waitFor(() => expect(screen.getByRole('button', { name: 'New secret' })).toHaveFocus())
    expect(screen.getByRole('button', { name: 'Import .env' })).toBeInTheDocument()
  })

  test('env with secrets: type its name to confirm; then the first remaining env is selected', async () => {
    const { user } = await openHotenb(await seeded())
    await user.click(within(nav()).getByRole('button', { name: 'Environment actions for Development' }))
    await user.click(await screen.findByRole('menuitem', { name: 'Delete environment' }))
    const dialog = await screen.findByRole('alertdialog', { name: 'Delete this environment?' })
    expect(dialog).toHaveTextContent("Deletes Development and its 8 secrets. This can't be undone.")
    const confirm = within(dialog).getByRole('button', { name: 'Delete environment' })
    expect(confirm).toBeDisabled()
    await user.type(within(dialog).getByLabelText('Type “Development” to confirm'), 'Development')
    await user.click(confirm)
    await table('Production')
    expect(within(nav()).queryByRole('button', { name: /^Development/ })).toBeNull()
  })

  test('env delete reads a fresh secret count (secrets added elsewhere require typing the name)', async () => {
    const m = createMockApi()
    const qa = await m.createEnvironment('p-hotenb', 'QA')
    const { user } = await openHotenb(m)
    await m.saveSecret(qa.id, 'ADDED_BY_CLI', 'x', 'text') // behind the screen's back
    await user.click(within(nav()).getByRole('button', { name: 'Environment actions for QA' }))
    await user.click(await screen.findByRole('menuitem', { name: 'Delete environment' }))
    const dialog = await screen.findByRole('alertdialog', { name: 'Delete this environment?' })
    expect(dialog).toHaveTextContent("Deletes QA and its 1 secret. This can't be undone.")
    expect(within(dialog).getByLabelText('Type “QA” to confirm')).toBeInTheDocument()
    expect(within(dialog).getByRole('button', { name: 'Delete environment' })).toBeDisabled()
  })

  test('empty env: a plain confirm with Cancel focused', async () => {
    const m = createMockApi()
    await m.createEnvironment('p-hotenb', 'QA')
    const { user } = await openHotenb(m)
    await user.click(within(nav()).getByRole('button', { name: 'Environment actions for QA' }))
    await user.click(await screen.findByRole('menuitem', { name: 'Delete environment' }))
    const dialog = await screen.findByRole('alertdialog', { name: 'Delete this environment?' })
    expect(dialog).toHaveTextContent("Deletes QA. This can't be undone.")
    expect(within(dialog).queryByRole('textbox')).toBeNull()
    await waitFor(() => expect(within(dialog).getByRole('button', { name: 'Cancel' })).toHaveFocus())
    await user.click(within(dialog).getByRole('button', { name: 'Delete environment' }))
    await waitFor(() => expect(within(nav()).queryByRole('button', { name: /^QA/ })).toBeNull())
  })

  test('a project with no environments: empty state with New environment', async () => {
    const m = createMockApi()
    const p = await m.createProject('fresh')
    const { user } = renderApp(m)
    await user.click(await screen.findByRole('button', { name: 'fresh' }))
    expect(await screen.findByText('No environments yet')).toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: 'New environment' })).toHaveLength(1)
    await user.click(screen.getByRole('button', { name: 'New environment' }))
    await screen.findByRole('dialog', { name: 'New environment' })
    await waitFor(() => expect(screen.getByLabelText('Name')).toHaveFocus())
    await user.keyboard('Development{Enter}')
    expect(await screen.findByText('No secrets in Development yet')).toBeInTheDocument()
    expect(app.screen).toMatchObject({ kind: 'project', projectId: p.id })
    // the empty-state trigger is gone: focus lands on the sidebar's New environment
    await waitFor(() => expect(within(nav()).getByRole('button', { name: 'New environment' })).toHaveFocus())
  })
})

describe('Project: environments', () => {
  test('switching env reloads the table', async () => {
    const m = await seeded()
    const spy = vi.spyOn(m, 'listSecrets')
    const { user } = await openHotenb(m)
    await user.click(envButton('Staging'))
    await table('Staging')
    expect(rowNames()).toEqual(['LOG_LEVEL', 'SENTRY_DSN'])
    expect(spy).toHaveBeenLastCalledWith(STAGING)
    expect(envButton('Staging')).toHaveAttribute('aria-current', 'page')
  })

  test('+ env: creates, selects it; a duplicate is shown inline', async () => {
    const { user } = await openHotenb(await seeded())
    await user.click(within(nav()).getByRole('button', { name: 'New environment' }))
    await screen.findByRole('dialog', { name: 'New environment' })
    await waitFor(() => expect(screen.getByLabelText('Name')).toHaveFocus())
    await user.keyboard('Staging{Enter}')
    await waitFor(() =>
      expect(screen.getByLabelText('Name')).toHaveAccessibleDescription('An environment with this name already exists'),
    )
    await user.clear(screen.getByLabelText('Name'))
    await user.keyboard('QA{Enter}')
    expect(await screen.findByText('No secrets in QA yet')).toBeInTheDocument()
    expect(envButton('QA')).toHaveAttribute('aria-current', 'page')
  })

  test('a stale secrets response is ignored', async () => {
    const m = await seeded()
    let release!: () => void
    let first = true
    const api: Api = {
      ...m,
      listSecrets: (envId) => {
        if (envId === DEV && first) {
          first = false
          return new Promise<SecretDTO[]>((r) => (release = () => void m.listSecrets(DEV).then(r)))
        }
        return m.listSecrets(envId)
      },
    }
    const { user } = renderApp(api)
    await user.click(await screen.findByRole('button', { name: 'hotenb' }))
    await user.click(await within(await screen.findByRole('navigation', { name: 'Environments' })).findByRole('button', { name: /^Staging/ }))
    await table('Staging')
    await act(async () => {
      release()
      await new Promise((r) => setTimeout(r, 0))
    })
    expect(screen.getByRole('table', { name: 'Secrets in Staging' })).toBeInTheDocument()
    expect(rowNames()).toEqual(['LOG_LEVEL', 'SENTRY_DSN'])
  })

  test('a project that no longer exists goes back to the projects', async () => {
    const m = createMockApi()
    renderApp(m)
    await screen.findByRole('heading', { level: 1, name: 'Projects' })
    act(() => app.dispatch({ type: 'OPEN_PROJECT', projectId: 'p-gone', envId: null }))
    await waitFor(() => expect(app.screen).toEqual({ kind: 'projects' }))
  })

  test('an error loading secrets shows an alert and Try again', async () => {
    const m = await seeded()
    let fail = true
    const api: Api = {
      ...m,
      listSecrets: (e) => (fail ? Promise.reject(new ApiError('internal', 'x')) : m.listSecrets(e)),
    }
    const { user } = renderApp(api)
    await user.click(await screen.findByRole('button', { name: 'hotenb' }))
    expect(await screen.findByText("Couldn't load secrets.")).toBeInTheDocument()
    noValueInPage()
    fail = false
    await user.click(screen.getByRole('button', { name: 'Try again' }))
    await table('Development')
  })

  test('loading: skeleton rows and a status', async () => {
    const m = await seeded()
    let release!: () => void
    const api: Api = {
      ...m,
      listSecrets: (e) => new Promise<SecretDTO[]>((r) => (release = () => void m.listSecrets(e).then(r))),
    }
    const { user } = renderApp(api)
    await user.click(await screen.findByRole('button', { name: 'hotenb' }))
    expect(await screen.findByText('Loading secrets')).toBeInTheDocument()
    await act(async () => release())
    await table('Development')
    expect(screen.queryByText('Loading secrets')).toBeNull()
  })
})

describe('Project: search and keyboard', () => {
  test('filters by name case-insensitively, Escape clears, no-match has a clear action', async () => {
    const { user } = await openHotenb(await seeded())
    const search = screen.getByRole('searchbox', { name: 'Search secrets' })
    await user.type(search, 'url')
    expect(rowNames()).toEqual(['DATABASE_URL', 'REDIS_URL'])
    await user.keyboard('{Escape}')
    expect(search).toHaveValue('')
    expect(rowNames()).toHaveLength(8)
    await user.type(search, 'zzz')
    expect(screen.getByText('No secrets match')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Clear search' }))
    expect(rowNames()).toHaveLength(8)
    expect(search).toHaveFocus()
  })

  test('Tab reaches one row; Enter and Space select; arrows move the selection', async () => {
    const { user } = await openHotenb(await seeded())
    const search = screen.getByRole('searchbox', { name: 'Search secrets' })
    search.focus()
    await user.tab() // Import .env
    expect(screen.getByRole('button', { name: 'Import .env' })).toHaveFocus()
    await user.tab() // New secret
    expect(screen.getByRole('button', { name: 'New secret' })).toHaveFocus()
    await user.tab()
    expect(row('API_KEY')).toHaveFocus()
    await user.keyboard('{Enter}')
    expect(row('API_KEY')).toHaveAttribute('aria-current', 'true')
    await user.keyboard('{ArrowDown}')
    expect(row('DATABASE_URL')).toHaveFocus()
    expect(row('DATABASE_URL')).toHaveAttribute('aria-current', 'true')
    await user.keyboard('{ArrowDown}{ArrowUp}')
    expect(row('DATABASE_URL')).toHaveFocus()
    await user.keyboard('{ArrowDown}')
    await user.keyboard(' ')
    expect(row('GITHUB_TOKEN')).toHaveAttribute('aria-current', 'true')
    // only the current row is in the tab order
    expect(rows().filter((r) => r.tabIndex === 0)).toEqual([row('GITHUB_TOKEN')])
    await user.tab() // into the row's actions
    expect(within(row('GITHUB_TOKEN')).getByRole('button', { name: 'Reveal GITHUB_TOKEN' })).toHaveFocus()
  })
})

describe('Project: large list', () => {
  test('500 secrets render one row each', async () => {
    const m = createMockApi()
    const env = (await m.createEnvironment('p-infra', 'Load')).id
    for (let i = 0; i < 500; i++) await m.saveSecret(env, `KEY_${String(i).padStart(3, '0')}`, `v${i}`, 'text')
    const { user } = renderApp(m)
    await user.click(await screen.findByRole('button', { name: 'infra' }))
    await user.click(await within(await screen.findByRole('navigation', { name: 'Environments' })).findByRole('button', { name: /^Load/ }))
    await table('Load')
    expect(rows()).toHaveLength(500)
    expect(new Set(rowNames()).size).toBe(500)
  })
})

describe('Project: import .env', () => {
  const FILE = { NEW_ONE: VALUE, JWT_SECRET: VALUE, NEW_TWO: VALUE }

  async function withFile() {
    const m = await seeded()
    m.__test.setFileContent(MOCK_IMPORT_PATH, FILE)
    return m
  }

  async function previewFrom(user: ReturnType<typeof renderApp>['user'], trigger: HTMLElement) {
    await user.click(trigger)
    const dialog = await screen.findByRole('dialog', { name: 'Import .env file' })
    await user.click(within(dialog).getByRole('button', { name: 'Choose file' }))
    await within(dialog).findByText('2 new, 1 already exists')
    return dialog
  }

  test('skip from the toolbar: new rows, counts, one toast, selection and reveal cleared, focus back', async () => {
    const m = await withFile()
    const { user } = await openHotenb(m)
    await user.click(row('SEEDED'))
    await user.click(within(detail()).getByRole('button', { name: 'Reveal' }))
    await waitFor(() => expect(revealed()).toHaveTextContent(VALUE))
    const trigger = screen.getByRole('button', { name: 'Import .env' })
    const dialog = await previewFrom(user, trigger)
    expect(revealed()).toBeNull()
    noValueInPage()
    await user.click(within(dialog).getByRole('button', { name: 'Import 2 secrets' }))
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Import .env file' })).not.toBeInTheDocument())
    await waitFor(() => expect(rowNames()).toEqual(expect.arrayContaining(['NEW_ONE', 'NEW_TWO'])))
    await waitFor(() => expect(envButton('Development')).toHaveAccessibleName('Development, 10 secrets'))
    expect(await screen.findByText('Imported 2 new, skipped 1')).toBeInTheDocument()
    expect(within(detail()).getByText('Select a secret to see its details.')).toBeInTheDocument()
    await waitFor(() => expect(trigger).toHaveFocus())
    noValueInPage()
    expect(await m.revealSecret(DEV, 'JWT_SECRET')).not.toBe(VALUE) // skipped
  })

  test('replace: the toast says replaced, the existing value changed in the vault only', async () => {
    const m = await withFile()
    const { user } = await openHotenb(m)
    const dialog = await previewFrom(user, screen.getByRole('button', { name: 'Import .env' }))
    await user.click(within(dialog).getByRole('radio', { name: 'Replace' }))
    await user.click(within(dialog).getByRole('button', { name: 'Import 3 secrets' }))
    expect(await screen.findByText('Imported 2 new, replaced 1')).toBeInTheDocument()
    expect(await m.revealSecret(DEV, 'JWT_SECRET')).toBe(VALUE)
    noValueInPage()
  })

  test('replace with nothing new: the toast says only what changed', async () => {
    const m = await seeded()
    m.__test.setFileContent(MOCK_IMPORT_PATH, { JWT_SECRET: VALUE })
    const { user } = await openHotenb(m)
    await user.click(screen.getByRole('button', { name: 'Import .env' }))
    const dialog = await screen.findByRole('dialog', { name: 'Import .env file' })
    await user.click(within(dialog).getByRole('button', { name: 'Choose file' }))
    await within(dialog).findByText('No new names, 1 already exists')
    await user.click(within(dialog).getByRole('radio', { name: 'Replace' }))
    await user.click(within(dialog).getByRole('button', { name: 'Import 1 secret' }))
    expect(await screen.findByText('Replaced 1')).toBeInTheDocument()
  })

  test('from an empty env: the empty state offers Import .env, then the table replaces it', async () => {
    const m = await withFile()
    await m.createEnvironment('p-hotenb', 'QA')
    const { user } = await openHotenb(m)
    await user.click(envButton('QA'))
    expect(await screen.findByText('No secrets in QA yet')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Import .env' }))
    const dialog = await screen.findByRole('dialog', { name: 'Import .env file' })
    await user.click(within(dialog).getByRole('button', { name: 'Choose file' }))
    await within(dialog).findByText('3 new')
    await user.click(within(dialog).getByRole('button', { name: 'Import 3 secrets' }))
    await table('QA')
    expect(rowNames()).toEqual(['JWT_SECRET', 'NEW_ONE', 'NEW_TWO'])
    expect(screen.queryByText('No secrets in QA yet')).toBeNull()
    expect(await screen.findByText('Imported 3 new')).toBeInTheDocument()
    // the empty-state trigger is gone: focus lands on New secret
    await waitFor(() => expect(screen.getByRole('button', { name: 'New secret' })).toHaveFocus())
    await waitFor(() => expect(envButton('QA')).toHaveAccessibleName('QA, 3 secrets'))
  })

  test('Cancel returns focus to the trigger and imports nothing', async () => {
    const m = await withFile()
    const { user } = await openHotenb(m)
    const trigger = screen.getByRole('button', { name: 'Import .env' })
    const dialog = await previewFrom(user, trigger)
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Import .env file' })).not.toBeInTheDocument())
    await waitFor(() => expect(trigger).toHaveFocus())
    expect(rowNames()).not.toContain('NEW_ONE')
  })

  test('locked mid-import: the unlock screen with the notice', async () => {
    const m = await withFile()
    const { user } = await openHotenb(m)
    const dialog = await previewFrom(user, screen.getByRole('button', { name: 'Import .env' }))
    m.__test.lockedByCli()
    await user.click(within(dialog).getByRole('button', { name: 'Import 2 secrets' }))
    await screen.findByRole('heading', { level: 1, name: 'Unlock your vault' })
    expect(screen.getByRole('status')).toHaveTextContent('Your vault was locked while you were working.')
    expect(screen.queryByRole('dialog', { name: 'Import .env file' })).not.toBeInTheDocument()
    await user.keyboard(`${MOCK_PASSWORD}{Enter}`)
    await table('Development')
    expect(rowNames()).not.toContain('NEW_ONE')
  })
})

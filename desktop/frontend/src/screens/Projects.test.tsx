import { screen, waitFor, within } from '@testing-library/react'
import { describe, expect, test, vi } from 'vitest'
import type { Api } from '@/api/client'
import { ApiError } from '@/api/errors'
import { createMockApi, type MockApi } from '@/api/mock'
import type { ProjectDTO } from '@/api/types'
import { app, renderApp } from '@/test/app'

const FIXTURE: ProjectDTO[] = [
  { id: 'p-b', name: 'billing', envCount: 1, secretCount: 2 },
  { id: 'p-a', name: 'Api-Gateway', envCount: 2, secretCount: 1 },
  { id: 'p-c', name: 'cdn', envCount: 0, secretCount: 0 },
]

/** The mock with a fixed project list (order as given, like the api). */
function withProjects(projects: ProjectDTO[], m: MockApi = createMockApi()): Api {
  return { ...m, listProjects: vi.fn(async () => projects) }
}

const list = () => screen.findByRole('list', { name: 'Projects' })
const card = (name: string) => screen.getByRole('button', { name })
const cardNames = () =>
  within(screen.getByRole('list', { name: 'Projects' }))
    .getAllByRole('listitem')
    .map((li) => li.querySelector('[data-project-name]')?.textContent)
const search = () => screen.getByRole('searchbox', { name: 'Search projects' })
const newButton = () => screen.getByRole('button', { name: 'New project' })

describe('Projects: list', () => {
  test('a skeleton while loading, then cards with name and counts in api order', async () => {
    let resolve!: (ps: ProjectDTO[]) => void
    const m = createMockApi()
    renderApp({ ...m, listProjects: () => new Promise<ProjectDTO[]>((r) => (resolve = r)) })
    expect(await screen.findByText('Loading projects')).toBeInTheDocument()
    resolve(FIXTURE)
    await list()
    expect(screen.queryByText('Loading projects')).not.toBeInTheDocument()
    expect(cardNames()).toEqual(['billing', 'Api-Gateway', 'cdn'])
    expect(card('billing')).toHaveAccessibleDescription('1 environment2 secrets')
    expect(card('billing')).toHaveTextContent('1 environment')
    expect(card('billing')).toHaveTextContent('2 secrets')
    expect(card('Api-Gateway')).toHaveTextContent('2 environments')
    expect(card('Api-Gateway')).toHaveTextContent('1 secret')
    expect(card('cdn')).toHaveTextContent('0 environments')
    expect(card('cdn')).toHaveTextContent('0 secrets')
  })

  test('a long name is truncated with its full text as title', async () => {
    const long = 'a-very-long-project-name-that-will-not-fit-in-one-card-at-all'
    renderApp(withProjects([{ id: 'p-l', name: long, envCount: 0, secretCount: 0 }]))
    await list()
    const name = screen.getByText(long)
    expect(name).toHaveAttribute('title', long)
    expect(name).toHaveClass('truncate')
  })

  test('the seeded vault lists its projects', async () => {
    renderApp(createMockApi())
    await list()
    expect(cardNames()).toEqual(['hotenb', 'infra', 'portfolio'])
    expect(card('infra')).toHaveTextContent('1 environment')
  })

  test('an empty vault shows the empty state with the primary action', async () => {
    const { user } = renderApp(withProjects([]))
    expect(await screen.findByText('No projects yet')).toBeInTheDocument()
    expect(screen.queryByRole('list', { name: 'Projects' })).not.toBeInTheDocument()
    // one primary action, not two
    expect(screen.getAllByRole('button', { name: 'New project' })).toHaveLength(1)
    await user.click(newButton())
    expect(await screen.findByRole('dialog', { name: 'New project' })).toBeInTheDocument()
  })

  test('an internal error shows an alert, "Try again" retries', async () => {
    const m = createMockApi()
    let fail = true
    const listProjects = vi.fn(() => (fail ? Promise.reject(new ApiError('internal', 'x')) : m.listProjects()))
    const { user } = renderApp({ ...m, listProjects })
    expect(await screen.findByRole('alert')).toHaveTextContent("Couldn't load projects.")
    fail = false
    await user.click(screen.getByRole('button', { name: 'Try again' }))
    await list()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(listProjects).toHaveBeenCalledTimes(2)
  })

  test('a locked error goes to unlock and shows no error alert', async () => {
    const m = createMockApi()
    const listProjects = vi.fn(() => {
      m.__test.lockedByCli()
      return m.listProjects()
    })
    renderApp({ ...m, listProjects })
    expect(await screen.findByRole('heading', { level: 1, name: 'Unlock your vault' })).toBeInTheDocument()
    expect(screen.queryByText("Couldn't load projects.")).not.toBeInTheDocument()
  })
})

describe('Projects: search', () => {
  test('filters case-insensitively, shows "No projects match" with a clear action', async () => {
    const { user } = renderApp(withProjects(FIXTURE))
    await list()
    await user.type(search(), 'API')
    expect(cardNames()).toEqual(['Api-Gateway'])
    await user.clear(search())
    await user.type(search(), 'zzz')
    expect(screen.getByText('No projects match')).toBeInTheDocument()
    expect(screen.queryByText('No projects yet')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Clear search' }))
    expect(search()).toHaveValue('')
    expect(search()).toHaveFocus()
    expect(cardNames()).toHaveLength(3)
  })

  test('Escape clears the search', async () => {
    const { user } = renderApp(withProjects(FIXTURE))
    await list()
    await user.type(search(), 'cdn')
    expect(cardNames()).toEqual(['cdn'])
    await user.keyboard('{Escape}')
    expect(search()).toHaveValue('')
    expect(cardNames()).toHaveLength(3)
  })
})

describe('Projects: opening a card', () => {
  test('a click opens the project', async () => {
    const { user } = renderApp(withProjects(FIXTURE))
    await list()
    await user.click(card('Api-Gateway'))
    expect(app.screen).toEqual({ kind: 'project', projectId: 'p-a', envId: null })
  })

  test.each(['{Enter}', ' '])('Tab to the card and press %s', async (key) => {
    const { user } = renderApp(withProjects(FIXTURE))
    await list()
    search().focus()
    await user.tab() // New project
    await user.tab() // first card
    expect(card('billing')).toHaveFocus()
    await user.tab() // its actions menu
    expect(screen.getByRole('button', { name: 'Project actions for billing' })).toHaveFocus()
    await user.tab()
    expect(card('Api-Gateway')).toHaveFocus()
    await user.tab({ shift: true })
    await user.tab({ shift: true })
    expect(card('billing')).toHaveFocus()
    await user.keyboard(key)
    expect(app.screen).toEqual({ kind: 'project', projectId: 'p-b', envId: null })
  })

  test('the actions menu is a sibling of the card button, not inside it', async () => {
    renderApp(withProjects(FIXTURE))
    await list()
    const button = card('billing')
    expect(button.querySelector('button, a, input, [tabindex]')).toBeNull()
    const trigger = screen.getByRole('button', { name: 'Project actions for billing' })
    expect(button.contains(trigger)).toBe(false)
    expect(button.closest('li')).toContainElement(trigger)
  })

  test('the actions menu opens with the keyboard and arrows move inside', async () => {
    const { user } = renderApp(withProjects(FIXTURE))
    await list()
    screen.getByRole('button', { name: 'Project actions for billing' }).focus()
    await user.keyboard('{Enter}')
    const item = await screen.findByRole('menuitem', { name: 'Delete project' })
    await user.keyboard('{ArrowDown}')
    await waitFor(() => expect(item).toHaveFocus())
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('menuitem')).not.toBeInTheDocument())
    screen.getByRole('button', { name: 'Project actions for billing' }).focus()
    await user.keyboard(' ')
    expect(await screen.findByRole('menuitem', { name: 'Delete project' })).toBeInTheDocument()
  })
})

describe('Projects: create', () => {
  test('creating closes the dialog, refreshes the list, refocuses "New project" and announces it', async () => {
    const m = createMockApi()
    const spy = vi.spyOn(m, 'listProjects')
    const { user } = renderApp(m)
    await list()
    await user.click(newButton())
    await screen.findByRole('dialog', { name: 'New project' })
    await waitFor(() => expect(screen.getByLabelText('Name')).toHaveFocus())
    await user.keyboard('billing{Enter}')
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'New project' })).not.toBeInTheDocument())
    await waitFor(() => expect(cardNames()).toContain('billing'))
    expect(spy).toHaveBeenCalledTimes(2)
    expect(newButton()).toHaveFocus()
    const announced = await screen.findByText('Project created')
    expect(announced.closest('[aria-live="polite"]')).not.toBeNull()
  })

  test('duplicate stays inline in the dialog', async () => {
    const { user } = renderApp(createMockApi())
    await list()
    await user.click(newButton())
    await screen.findByRole('dialog', { name: 'New project' })
    const input = screen.getByLabelText('Name')
    await waitFor(() => expect(input).toHaveFocus())
    await user.keyboard('hotenb{Enter}')
    await waitFor(() => expect(input).toHaveAccessibleDescription('A project with this name already exists'))
    expect(input).toHaveValue('hotenb')
  })
})

describe('Projects: delete', () => {
  async function openDelete(user: ReturnType<typeof renderApp>['user'], name: string) {
    await user.click(screen.getByRole('button', { name: `Project actions for ${name}` }))
    await user.click(await screen.findByRole('menuitem', { name: 'Delete project' }))
    return screen.findByRole('alertdialog', { name: 'Delete this project?' })
  }

  test('end to end: names the project and counts, type the name, confirm', async () => {
    const m = createMockApi()
    const del = vi.spyOn(m, 'deleteProject')
    const { user } = renderApp(m)
    await list()
    const dialog = await openDelete(user, 'portfolio')
    expect(dialog).toHaveTextContent("Deletes portfolio, its 2 environments and 5 secrets. This can't be undone.")
    const confirm = within(dialog).getByRole('button', { name: 'Delete project' })
    expect(confirm).toBeDisabled()
    await user.type(within(dialog).getByLabelText('Type portfolio to confirm'), 'portfolio')
    await user.click(confirm)
    await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
    expect(del).toHaveBeenCalledWith('p-portfolio')
    await waitFor(() => expect(cardNames()).toEqual(['hotenb', 'infra']))
    expect(newButton()).toHaveFocus()
  })

  test('cancel returns focus to the actions button', async () => {
    const { user } = renderApp(createMockApi())
    await list()
    await openDelete(user, 'infra')
    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
    await waitFor(() => expect(screen.getByRole('button', { name: 'Project actions for infra' })).toHaveFocus())
  })

  test('deleting the last project shows the empty state', async () => {
    const m = createMockApi()
    const { user } = renderApp(m)
    await list()
    // the others go away behind the screen's back (e.g. the CLI): infra is the last one
    for (const p of await m.listProjects()) if (p.name !== 'infra') await m.deleteProject(p.id)
    await openDelete(user, 'infra')
    await user.type(screen.getByLabelText('Type infra to confirm'), 'infra{Enter}')
    expect(await screen.findByText('No projects yet')).toBeInTheDocument()
    await waitFor(() => expect(newButton()).toHaveFocus())
  })
})

describe('Projects: accessibility', () => {
  test('one h1, a labelled search, named icon buttons', async () => {
    renderApp(withProjects(FIXTURE))
    await list()
    expect(screen.getAllByRole('heading', { level: 1 })).toHaveLength(1)
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Projects')
    expect(search()).toBeInTheDocument()
    for (const b of screen.getAllByRole('button')) expect(b).toHaveAccessibleName()
  })
})

import { render, screen } from '@testing-library/react'
import { describe, expect, test, vi } from 'vitest'
import { API_METHODS } from '@/api/client'
import { createMockApi } from '@/api/mock'
import App from '@/App'
import { renderApp } from '@/test/app'

describe('App', () => {
  test('backend unavailable: fatal screen, no api call, no action', async () => {
    const m = createMockApi()
    for (const k of API_METHODS) vi.spyOn(m, k)
    render(<App api={m} backendStatus="unavailable" />)
    expect(await screen.findByRole('heading', { level: 1, name: "Secrio can't reach its backend" })).toBeInTheDocument()
    expect(screen.getByText('Restart the app.')).toBeInTheDocument()
    expect(screen.queryByRole('button')).toBeNull()
    expect(API_METHODS.filter((k) => vi.mocked(m[k]).mock.calls.length > 0)).toEqual([])
  })

  describe('accessibility smoke', () => {
    test.each([
      ['unlock', () => createMockApi({ locked: true }), 'Unlock your vault'],
      ['create vault', () => createMockApi({ noVault: true }), 'Create your first vault'],
      ['projects shell', () => createMockApi(), 'Projects'],
    ])('%s: one h1, named inputs and buttons', async (_n, make, title) => {
      renderApp(make())
      await screen.findByRole('heading', { level: 1, name: title })
      expect(screen.getAllByRole('heading', { level: 1 })).toHaveLength(1)
      for (const input of document.querySelectorAll('input')) expect(input).toHaveAccessibleName()
      for (const b of screen.getAllByRole('button')) expect(b).toHaveAccessibleName()
      expect(screen.getByRole('main')).toBeInTheDocument()
    })
  })
})

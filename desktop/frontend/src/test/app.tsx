// Renders the whole app over an injected api (usually createMockApi).
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useEffect } from 'react'
import type { Api } from '@/api/client'
import type { BackendStatus } from '@/api/select'
import { Shell } from '@/App'
import { AppProvider, useApp } from '@/state/AppProvider'

export let app: ReturnType<typeof useApp>

/** Exposes the app context to the test (screen, dispatch). */
function Probe() {
  const v = useApp()
  useEffect(() => {
    app = v
  })
  return null
}

export function renderApp(api: Api, backendStatus: BackendStatus = 'mock') {
  const user = userEvent.setup()
  const r = render(
    <AppProvider api={api} backendStatus={backendStatus}>
      <Shell />
      <Probe />
    </AppProvider>,
  )
  return { user, ...r }
}

export const h1 = () => screen.findByRole('heading', { level: 1 })
export const passwordInput = () => screen.getByLabelText<HTMLInputElement>('Password')

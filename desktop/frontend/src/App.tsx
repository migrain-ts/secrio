import type { Api } from '@/api/client'
import type { BackendStatus } from '@/api/select'
import { TopBar } from '@/components/TopBar'
import { Toaster } from '@/components/ui/toaster'
import { CreateVault } from '@/screens/CreateVault'
import { Fatal } from '@/screens/Fatal'
import { Project } from '@/screens/Project'
import { Projects } from '@/screens/Projects'
import { Unlock } from '@/screens/Unlock'
import { AppProvider, useApp } from '@/state/AppProvider'

/** Props only for tests: production uses the real api and backendStatus. */
export default function App(props: { api?: Api; backendStatus?: BackendStatus }) {
  return (
    <AppProvider {...props}>
      <Shell />
    </AppProvider>
  )
}

/** Renders the current screen. */
export function Shell() {
  const { screen } = useApp()
  switch (screen.kind) {
    case 'boot':
      // Usually one quick call: nothing to show rather than a flashing spinner.
      return <main aria-busy="true" className="min-h-screen bg-background" />
    case 'fatal':
      return <Fatal />
    case 'unlock':
      // keyed so a new lock starts a fresh form (no stale error/password)
      return <Unlock key={`${screen.vaultId}:${screen.notice ?? ''}`} vaultId={screen.vaultId} notice={screen.notice} />
    case 'create-vault':
      return <CreateVault />
    case 'projects':
    case 'project':
      return (
        <div className="flex h-screen flex-col bg-background">
          <TopBar />
          <main className="flex-1 overflow-auto">
            {screen.kind === 'projects' ? (
              <Projects />
            ) : (
              <Project key={screen.projectId} projectId={screen.projectId} envId={screen.envId} />
            )}
          </main>
          {/* transient confirmations ("Project created"), polite live region */}
          <Toaster />
        </div>
      )
  }
}

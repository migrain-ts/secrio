// The one place a secret value lives in the UI: a single piece of state for
// the selected secret, fetched on a deliberate click and dropped as soon as
// anything changes. Never stored, logged, or put in a title/label/toast.
import { useCallback, useEffect, useRef, useState } from 'react'
import type { Api } from '@/api/client'
import { ApiError } from '@/api/errors'

/** How long a revealed value stays on screen. */
export const REVEAL_MS = 10_000

interface Revealed {
  envId: string
  name: string
  value: string
}

export interface Reveal {
  /** The selected secret's value while revealed, else null. */
  value: string | null
  /** A reveal request is in flight. */
  pending: boolean
  /** The last reveal failed (short fixed copy in the UI, never server text). */
  failed: boolean
  /** Fetches the selected secret's value. */
  reveal(): Promise<void>
  /** Drops the value now (copy, edit and delete call it before they start). */
  hide(): void
}

/**
 * Hidden again when REVEAL_MS pass, the selection or env changes, the window
 * loses focus or becomes hidden, hide() is called, or the component unmounts
 * (a lock leaves the screen, so it unmounts too).
 */
export function useReveal(api: Api, envId: string | null, name: string | null, onNotFound: () => void): Reveal {
  const [revealed, setRevealed] = useState<Revealed | null>(null)
  const [pending, setPending] = useState(false)
  const [failed, setFailed] = useState(false)
  // Bumped by every hide: a request started before it is ignored.
  const seq = useRef(0)
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)

  const hide = useCallback(() => {
    seq.current++
    clearTimeout(timer.current)
    timer.current = undefined
    setRevealed(null)
    setPending(false)
    setFailed(false)
  }, [])

  // Any change of target drops the value (and any request in flight).
  useEffect(() => hide, [envId, name, hide])

  useEffect(() => {
    const onVisibility = () => {
      if (document.visibilityState === 'hidden') hide()
    }
    window.addEventListener('blur', hide)
    document.addEventListener('visibilitychange', onVisibility)
    return () => {
      window.removeEventListener('blur', hide)
      document.removeEventListener('visibilitychange', onVisibility)
    }
  }, [hide])

  const notFound = useRef(onNotFound)
  useEffect(() => {
    notFound.current = onNotFound
  })

  const reveal = useCallback(async () => {
    if (envId === null || name === null) return
    hide()
    const mine = seq.current
    setPending(true)
    try {
      const value = await api.revealSecret(envId, name)
      if (mine !== seq.current) return
      setRevealed({ envId, name, value })
      timer.current = setTimeout(hide, REVEAL_MS)
    } catch (e) {
      if (mine !== seq.current) return
      setFailed(true)
      if (e instanceof ApiError && e.code === 'not_found') notFound.current()
    } finally {
      if (mine === seq.current) setPending(false)
    }
  }, [api, envId, name, hide])

  const value = revealed && revealed.envId === envId && revealed.name === name ? revealed.value : null
  return { value, pending, failed, reveal, hide }
}

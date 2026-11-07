'use client'

import { useEffect, useRef } from 'react'
import { usePathname } from 'next/navigation'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { profileQueryOptions, useRenewSession } from '@/features/auth/hooks/useAuth'

/** How long a tab waits before re-asking after a renewal attempt that left the flag set. */
export const SESSION_RENEWAL_RETRY_MS = 5 * 60 * 1000

/**
 * How often a tab that stays open re-reads the profile, so the renewal flag
 * reaches it without a page load. Daily use through client navigation alone
 * would otherwise never see the flag again, and the session would end on day
 * sixty despite the visits.
 */
export const SESSION_RENEWAL_POLL_MS = 6 * 60 * 60 * 1000

/**
 * Restarts the signed-in session's clock when the viewer comes back.
 *
 * The backend end is `sessionRenewalDue` (backend/internal/api/handlers/auth):
 * the profile every page load reads says `session_renewal_due` once the
 * session is older than a day, and it is the profile that has to say so,
 * because on a cold load the profile is fetched by the Next server, which
 * drops a Set-Cookie the backend might write. This posts to /auth/refresh when
 * the flag is set; the backend answers with a fresh auth cookie carrying the
 * full session lifetime. Nothing renders.
 *
 * Asking is bounded two ways. A ref holds the time of the last attempt, so
 * React Strict Mode's doubled effect, a profile refetch that still says due,
 * and a failed attempt cannot send another request inside
 * SESSION_RENEWAL_RETRY_MS; `useRenewSession` marks the cached profile as not
 * due on success, which resets the ref, so the next profile read that says due
 * again (a later day) asks again. The two are not redundant.
 *
 * Not on /auth pages: those run their own session changes on mount (magic
 * link, recovery, password reset, the Google completion step), and a renewal
 * answering after one of them would put the previous session's cookie back.
 * Click-started session changes elsewhere wait for a renewal in flight
 * instead (`settlePendingSessionRenewal`).
 */
export function SessionRenewal() {
  const pathname = usePathname()
  const queryClient = useQueryClient()
  const { data: profile } = useQuery({
    ...profileQueryOptions(queryClient),
    refetchInterval: SESSION_RENEWAL_POLL_MS,
    refetchIntervalInBackground: false,
  })
  const { mutate: renew } = useRenewSession()
  const lastAttemptAt = useRef<number | null>(null)

  const onAuthSurface = pathname?.startsWith('/auth') === true
  const due = profile?.session_renewal_due === true

  useEffect(() => {
    if (!due) {
      lastAttemptAt.current = null
      return
    }
    if (onAuthSurface) return
    const now = Date.now()
    if (lastAttemptAt.current != null && now - lastAttemptAt.current < SESSION_RENEWAL_RETRY_MS) return
    lastAttemptAt.current = now
    renew()
  }, [profile, due, onAuthSurface, renew])

  return null
}

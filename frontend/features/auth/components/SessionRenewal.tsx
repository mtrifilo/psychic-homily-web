'use client'

import { useEffect, useRef } from 'react'
import { usePathname } from 'next/navigation'
import { useIsMutating, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  SESSION_CHANGE_MUTATION_KEY,
  profileQueryOptions,
  useRenewSession,
  type UserProfile,
} from '@/features/auth/hooks/useAuth'

/** How long a tab waits before re-asking after a renewal attempt. */
export const SESSION_RENEWAL_RETRY_MS = 5 * 60 * 1000

/**
 * How often a signed-in tab that stays open re-reads the profile, so the
 * renewal flag reaches it without a page load. Daily use through client
 * navigation alone would otherwise never see the flag again, and the session
 * would end when the lifetime ran out despite the visits.
 */
export const SESSION_RENEWAL_POLL_MS = 6 * 60 * 60 * 1000

/**
 * The poll runs only for a signed-in viewer: an anonymous tab has nothing to
 * renew, and polling the protected profile would only earn it a 401.
 */
export function sessionRenewalPollInterval(profile: UserProfile | undefined): number | false {
  return profile?.success === true && profile.user != null ? SESSION_RENEWAL_POLL_MS : false
}

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
 * Asking is bounded by time: a ref holds the last attempt, so React Strict
 * Mode's doubled effect, a profile refetch that still says due, and a failed
 * attempt cannot send another request inside SESSION_RENEWAL_RETRY_MS, while
 * a later profile read that says due again (the next day) can.
 * `useRenewSession` marks the cached profile as not due on success, which
 * is what stops the asking in between; the two are not redundant.
 *
 * Never under a session change. On /auth pages the change runs on mount
 * (magic link, recovery, password reset, the Google completion step, the
 * passkey finishes), so the effect stays off there; elsewhere a change in
 * flight (`SESSION_CHANGE_MUTATION_KEY`) holds the effect, and a change that
 * starts after a renewal waits for it (`settlePendingSessionRenewal`). Either
 * way the change's cookie lands last.
 */
export function SessionRenewal() {
  const pathname = usePathname()
  const queryClient = useQueryClient()
  const { data: profile } = useQuery({
    ...profileQueryOptions(queryClient),
    refetchInterval: query => sessionRenewalPollInterval(query.state.data),
    refetchIntervalInBackground: false,
  })
  const { mutate: renew } = useRenewSession()
  const sessionChanging = useIsMutating({ mutationKey: SESSION_CHANGE_MUTATION_KEY }) > 0
  const lastAttemptAt = useRef<number | null>(null)

  const onAuthSurface = pathname?.startsWith('/auth') === true
  const due = profile?.session_renewal_due === true

  useEffect(() => {
    if (!due || onAuthSurface || sessionChanging) return
    const now = Date.now()
    if (lastAttemptAt.current != null && now - lastAttemptAt.current < SESSION_RENEWAL_RETRY_MS) return
    lastAttemptAt.current = now
    renew()
  }, [profile, due, onAuthSurface, sessionChanging, renew])

  return null
}

'use client'

import { useEffect, useRef } from 'react'
import { useProfile, useRefreshToken } from '@/features/auth/hooks/useAuth'

/**
 * Restarts the signed-in session's clock when the viewer comes back.
 *
 * The profile every page load reads (seeded by the server render, so this
 * works on a cold load too) says `session_renewal_due` once the session is
 * older than a day. This posts to /auth/refresh exactly once per page load in
 * that case; the backend answers with a fresh auth cookie carrying the full
 * session lifetime. Nothing renders, and a failed renewal is left alone: the
 * session the viewer has keeps working, and the next load asks again.
 *
 * Once per page load, not per flag change: the latch is a ref so React Strict
 * Mode's doubled effect and a profile refetch that still says due cannot send
 * a second request. `useRefreshToken` separately marks the cached profile as
 * no longer due on success, which covers a remount; the two are not redundant.
 *
 * The flag is trusted as sent: the backend sets it only on a successful
 * profile for a session credential, and the server-rendered seed carries it
 * through unchanged.
 */
export function SessionRenewal() {
  const { data: profile } = useProfile()
  const { mutate: renew } = useRefreshToken()
  const attempted = useRef(false)

  const due = profile?.session_renewal_due === true

  useEffect(() => {
    if (!due || attempted.current) return
    attempted.current = true
    renew()
  }, [due, renew])

  return null
}

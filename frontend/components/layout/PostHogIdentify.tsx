'use client'

import { useEffect } from 'react'
import { useCookieConsent } from '@/lib/context/CookieConsentContext'
import { useAuthContext } from '@/lib/context/AuthContext'
import { identifyUser, resetAnalytics } from '@/lib/posthog'

/**
 * Ties the analytics identity to the signed-in viewer.
 *
 * Split out of `<PostHogProvider>` so the only auth-aware piece of the
 * analytics stack is the piece that actually needs auth. The provider itself
 * stays above the auth hydration boundary, which keeps consent handling — and
 * the cookie banner it wraps — off the critical path of the profile prefetch;
 * this component renders below the boundary, inside `AuthProvider`.
 *
 * Renders nothing.
 */
export function PostHogIdentify(): null {
  const { canUseAnalytics } = useCookieConsent()
  const { user, isAuthenticated } = useAuthContext()

  useEffect(() => {
    if (!canUseAnalytics) return
    if (isAuthenticated && user) {
      identifyUser(user.id, {
        email: user.email,
        is_admin: user.is_admin,
      })
    } else {
      resetAnalytics()
    }
  }, [isAuthenticated, user, canUseAnalytics])

  return null
}

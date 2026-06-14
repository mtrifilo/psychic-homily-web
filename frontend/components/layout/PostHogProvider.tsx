'use client'

import { Suspense, useEffect } from 'react'
import { usePathname, useSearchParams } from 'next/navigation'
import { useCookieConsent } from '@/lib/context/CookieConsentContext'
import { useAuthContext } from '@/lib/context/AuthContext'
import {
  enableAnalytics,
  disableAnalytics,
  capturePageview,
  identifyUser,
  resetAnalytics,
} from '@/lib/posthog'

// Separate component for search params tracking to allow Suspense boundary
function PostHogPageView(): null {
  const pathname = usePathname()
  const searchParams = useSearchParams()
  const { canUseAnalytics } = useCookieConsent()

  // Track pageviews (no-op until posthog has lazy-loaded, i.e. after consent)
  useEffect(() => {
    if (!canUseAnalytics) return
    capturePageview(window.location.href)
  }, [pathname, searchParams, canUseAnalytics])

  return null
}

export function PostHogProvider({ children }: { children: React.ReactNode }) {
  const { canUseAnalytics, isLoaded } = useCookieConsent()
  const { user, isAuthenticated } = useAuthContext()

  // Lazy-load + opt in only once analytics consent is granted; opt out
  // otherwise. posthog-js is never fetched for visitors who don't consent
  // (PSY-1091 — keeps it off the eager critical path). enableAnalytics is
  // idempotent, so re-running on every consent-sync render is safe.
  useEffect(() => {
    if (!isLoaded) return
    if (canUseAnalytics) {
      void enableAnalytics()
    } else {
      disableAnalytics()
    }
  }, [canUseAnalytics, isLoaded])

  // Identify authenticated users (no-op until posthog has loaded post-consent)
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

  return (
    <>
      <Suspense fallback={null}>
        <PostHogPageView />
      </Suspense>
      {children}
    </>
  )
}

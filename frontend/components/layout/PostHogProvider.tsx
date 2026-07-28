'use client'

import { Suspense, useEffect } from 'react'
import { usePathname, useSearchParams } from 'next/navigation'
import { useCookieConsent } from '@/lib/context/CookieConsentContext'
import {
  enableAnalytics,
  disableAnalytics,
  capturePageview,
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

// Consent-driven analytics lifecycle. Intentionally auth-UNAWARE: it renders
// above the auth hydration boundary so the cookie-consent banner it wraps is
// never gated on the server-side profile prefetch. Tying the analytics
// identity to the signed-in viewer is `<PostHogIdentify>`'s job, mounted below
// that boundary.
export function PostHogProvider({ children }: { children: React.ReactNode }) {
  const { canUseAnalytics, isLoaded } = useCookieConsent()

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

  return (
    <>
      <Suspense fallback={null}>
        <PostHogPageView />
      </Suspense>
      {children}
    </>
  )
}

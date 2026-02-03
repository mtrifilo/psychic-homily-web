'use client'

import { Suspense, useEffect, useRef } from 'react'
import { usePathname, useSearchParams } from 'next/navigation'
import { useCookieConsent } from '@/lib/context/CookieConsentContext'
import { useAuthContext } from '@/lib/context/AuthContext'
import {
  initPostHog,
  optInPostHog,
  optOutPostHog,
  posthog,
} from '@/lib/posthog'

// Separate component for search params tracking to allow Suspense boundary
function PostHogPageView() {
  const pathname = usePathname()
  const searchParams = useSearchParams()
  const { canUseAnalytics } = useCookieConsent()

  // Track pageviews
  useEffect(() => {
    if (!canUseAnalytics) return
    posthog.capture('$pageview', { $current_url: window.location.href })
  }, [pathname, searchParams, canUseAnalytics])

  return null
}

export function PostHogProvider({ children }: { children: React.ReactNode }) {
  const { canUseAnalytics, isLoaded } = useCookieConsent()
  const { user, isAuthenticated } = useAuthContext()
  const prevConsent = useRef<boolean | null>(null)

  // Initialize on mount (doesn't start tracking)
  useEffect(() => {
    initPostHog()
  }, [])

  // Handle consent changes
  useEffect(() => {
    if (!isLoaded || prevConsent.current === canUseAnalytics) return
    prevConsent.current = canUseAnalytics
    canUseAnalytics ? optInPostHog() : optOutPostHog()
  }, [canUseAnalytics, isLoaded])

  // Identify authenticated users
  useEffect(() => {
    if (!canUseAnalytics) return
    if (isAuthenticated && user) {
      posthog.identify(user.id, {
        email: user.email,
        is_admin: user.is_admin,
      })
    } else {
      posthog.reset()
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

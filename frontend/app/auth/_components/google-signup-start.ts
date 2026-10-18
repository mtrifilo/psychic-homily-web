'use client'

import { useEffect, useRef, useState } from 'react'
// Direct backend origin, never the Next.js /api proxy: OAuth is a full-page
// redirect and the redirect chain does not survive the proxy. See
// lib/api-base.ts for the resolution order.
import { OAUTH_BACKEND_URL } from '@/lib/api-base'
import { FALLBACK_RETURN_TO } from '@/lib/auth-href'
import { useHydrated } from '@/lib/hooks/common/useHydrated'
import { MIN_SIGNUP_AGE } from '@/lib/legal'
import {
  useConsentVersions,
  type ConsentVersions,
} from '@/features/auth/hooks/useLegalVersions'

/**
 * The backend route that starts a Google sign-in, and with `signup` a sign-up
 * that accepts those versions and attests MIN_SIGNUP_AGE. The route reads
 * `returnTo` (`oauthReturnToParam` in
 * backend/internal/api/handlers/auth/oauth_return_to.go), checks it again,
 * records it for its callback, and lands on the site root for anything it
 * refuses.
 */
export function googleStartUrl({
  returnTo,
  signup,
}: {
  returnTo: string
  signup?: ConsentVersions
}): string {
  const url = new URL(`${OAUTH_BACKEND_URL}/auth/login/google`)
  if (signup) {
    url.searchParams.set('signup_intent', '1')
    url.searchParams.set('terms_accepted', 'true')
    url.searchParams.set('terms_version', signup.termsVersion)
    url.searchParams.set('privacy_version', signup.privacyVersion)
    url.searchParams.set('age_confirmed', 'true')
    url.searchParams.set('min_age_attested', String(MIN_SIGNUP_AGE))
  }
  if (returnTo !== FALLBACK_RETURN_TO) {
    url.searchParams.set('returnTo', returnTo)
  }
  return url.toString()
}

// One Google sign-up start, made from the sign-up form in this tab, whose
// consent refusal has not been retried yet. Holds the start's time in ms.
export const GOOGLE_SIGNUP_START_KEY = 'psy:consent-retry:google-signup'

// The lifetime of the signup consent cookie the backend sets for the OAuth
// round trip (600 s in backend/internal/api/handlers/auth/oauth_handlers.go).
// A refusal arriving later than that does not answer this start.
export const GOOGLE_SIGNUP_START_TTL_MS = 10 * 60 * 1000

/** Records a sign-up start the auth page may retry once. */
export function rememberGoogleSignupStart(): void {
  try {
    sessionStorage.setItem(GOOGLE_SIGNUP_START_KEY, String(Date.now()))
  } catch {
    // Without storage the refusal is shown instead of retried.
  }
}

/** Drops any recorded start, so no later refusal retries it. */
export function forgetGoogleSignupStart(): void {
  try {
    sessionStorage.removeItem(GOOGLE_SIGNUP_START_KEY)
  } catch {
    // Nothing could have been recorded either.
  }
}

function hasRetryableGoogleSignupStart(): boolean {
  try {
    const startedAt = Number(sessionStorage.getItem(GOOGLE_SIGNUP_START_KEY))
    const age = Date.now() - startedAt
    return startedAt > 0 && age >= 0 && age < GOOGLE_SIGNUP_START_TTL_MS
  } catch {
    return false
  }
}

/**
 * - `none`: no retry; the page shows the refusal it arrived with.
 * - `retrying`: the start is being sent again with freshly read versions.
 * - `unavailable`: the versions for the retry could not be read.
 */
export type GoogleSignupRetry = 'none' | 'retrying' | 'unavailable'

/**
 * Retries, once, a Google sign-up start refused for its consent versions.
 *
 * The backend answers a start whose versions it does not accept, and a
 * callback that would create an account without accepted consent, by
 * redirecting to the auth page with TERMS_ACCEPTANCE_REQUIRED. When that
 * arrival answers a start the sign-up form recorded in this tab within
 * GOOGLE_SIGNUP_START_TTL_MS, the versions are read again past every cache
 * and the start is sent again, consent and returnTo included. Any other
 * arrival, a signed-in one or a sign-in from "Continue with Google" among
 * them, keeps its refusal, and every arrival drops the record, as does a
 * return to the page from the back-forward cache.
 */
export function useGoogleSignupRetry({
  arrival,
  consentArrival,
  returnTo,
}: {
  /** The arrival's query string. An outcome lasts only for its arrival. */
  arrival: string
  /** The arrival is a TERMS_ACCEPTANCE_REQUIRED refusal. */
  consentArrival: boolean
  returnTo: string
}): GoogleSignupRetry {
  const consentVersions = useConsentVersions()
  // The record stays until the retry leaves the page or fails, so this holds
  // for the whole retry. Read only after hydration: the server has no storage.
  const hydrated = useHydrated()
  const startRecorded =
    hydrated && consentArrival && hasRetryableGoogleSignupStart()
  // The arrival whose retry could not read the versions.
  const [unavailableArrival, setUnavailableArrival] = useState<string | null>(
    null
  )
  const mountedRef = useRef(false)
  const retryInFlightRef = useRef(false)

  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
    }
  }, [])

  // A page restored from the back-forward cache runs no effects, and the
  // reader who went back from the provider abandoned that start.
  useEffect(() => {
    const dropOnRestore = (event: PageTransitionEvent) => {
      if (event.persisted) forgetGoogleSignupStart()
    }
    window.addEventListener('pageshow', dropOnRestore)
    return () => window.removeEventListener('pageshow', dropOnRestore)
  }, [])

  useEffect(() => {
    if (!consentArrival) {
      forgetGoogleSignupStart()
      return
    }
    if (retryInFlightRef.current) {
      return
    }
    if (!hasRetryableGoogleSignupStart()) {
      forgetGoogleSignupStart()
      return
    }
    retryInFlightRef.current = true
    consentVersions
      .refresh()
      .then(
        versions => {
          // Forgotten before the retry leaves, so the retry's own refusal finds
          // no start to retry: the one-attempt bound.
          forgetGoogleSignupStart()
          // A page hidden or left meanwhile shows its refusal on return rather
          // than navigating on its own: showing it renders again, and the
          // record is gone by then.
          if (mountedRef.current) {
            window.location.href = googleStartUrl({ returnTo, signup: versions })
          }
        },
        () => {
          forgetGoogleSignupStart()
          setUnavailableArrival(arrival)
        }
      )
      .finally(() => {
        retryInFlightRef.current = false
      })
  }, [arrival, consentArrival, consentVersions, returnTo])

  if (unavailableArrival === arrival) {
    return 'unavailable'
  }
  return startRecorded ? 'retrying' : 'none'
}

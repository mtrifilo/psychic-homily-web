'use client'

import { useEffect, useRef, useState, useSyncExternalStore } from 'react'
// Direct backend origin, never the Next.js /api proxy: OAuth is a full-page
// redirect and the redirect chain does not survive the proxy. See
// lib/api-base.ts for the resolution order (PSY-1649).
import { OAUTH_BACKEND_URL } from '@/lib/api-base'
import { FALLBACK_RETURN_TO } from '@/lib/auth-href'
import { MIN_SIGNUP_AGE } from '@/lib/legal'
import {
  useConsentVersions,
  type ConsentVersions,
} from '@/features/auth/hooks/useLegalVersions'

/** What a Google sign-up start carries for the account it may create. */
export interface GoogleSignupConsent {
  versions: ConsentVersions
  minAge?: number
}

/**
 * The backend route that starts a Google sign-in, and with `signup` a sign-up.
 * The route reads `returnTo` (`oauthReturnToParam` in
 * backend/internal/api/handlers/auth/oauth_return_to.go), checks it again,
 * records it for its callback, and lands on the site root for anything it
 * refuses.
 */
export function googleStartUrl({
  returnTo,
  signup,
}: {
  returnTo: string
  signup?: GoogleSignupConsent
}): string {
  const url = new URL(`${OAUTH_BACKEND_URL}/auth/login/google`)
  if (signup) {
    url.searchParams.set('signup_intent', '1')
    url.searchParams.set('terms_accepted', 'true')
    url.searchParams.set('terms_version', signup.versions.termsVersion)
    url.searchParams.set('privacy_version', signup.versions.privacyVersion)
    url.searchParams.set('age_confirmed', 'true')
    if (signup.minAge) {
      url.searchParams.set('min_age_attested', String(signup.minAge))
    }
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

// The record changes only in this tab's own handlers, each of which also
// changes React state or leaves the page, so nothing needs to subscribe.
function subscribeToNothing(): () => void {
  return () => {}
}

function noRecordOnServer(): boolean {
  return false
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
 * arrival, a sign-in from "Continue with Google" among them, keeps its
 * refusal, and every arrival drops the record.
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
  // for the whole retry.
  const startRecorded = useSyncExternalStore(
    subscribeToNothing,
    hasRetryableGoogleSignupStart,
    noRecordOnServer
  )
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
          // than navigating on its own.
          if (mountedRef.current) {
            window.location.href = googleStartUrl({
              returnTo,
              signup: { versions, minAge: MIN_SIGNUP_AGE },
            })
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
  return consentArrival && startRecorded ? 'retrying' : 'none'
}

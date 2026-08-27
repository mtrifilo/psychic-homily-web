'use client'

import { useCallback, useEffect, useState } from 'react'
import type { ApiError } from '@/lib/api'

/**
 * How long the resend control stays parked after a successful send.
 *
 * Matched to the backend's `Retry-After: 60` on the throttled resend endpoint
 * (`rateLimitHandler` in backend/internal/api/routes/shared.go) so the two
 * never disagree about how long "in a minute" is. The server budget is 5/min
 * per IP, so this is the deliberate UX throttle from the approved mocks, not a
 * mirror of the server's own counter.
 */
export const VERIFICATION_RESEND_COOLDOWN_SECONDS = 60

/**
 * Whether a running wait's length is something the app actually knows.
 *
 * `exact` covers the two cases where the deadline is real: the client-side
 * cooldown this module owns after a successful send, and a 429 whose
 * `Retry-After` the browser let us read. `approximate` covers a 429 that
 * arrived with no readable `Retry-After` — the control still parks, but the UI
 * must not quote a second count it invented.
 *
 * The distinction is load-bearing rather than theoretical: in PRODUCTION the
 * `Retry-After` header is not exposed cross-origin (see the note on the 429
 * branch of `apiRequest` in lib/api.ts), so `ApiError.retryAfter` is undefined
 * for every throttled resend a real user hits. Until that is fixed backend-side
 * (PSY-1924), `approximate` is the live path and `exact` is the local-dev one.
 */
export type ResendWaitPrecision = 'exact' | 'approximate'

/** A throttled resend's wait, and how much we trust its length. */
export interface VerificationResendThrottle {
  seconds: number
  precision: ResendWaitPrecision
}

/**
 * Reads the wait a throttled resend asks for.
 *
 * Returns `null` for anything that is not a throttle, so callers can keep the
 * "please wait" state and the "that failed" state apart: a 429 is a normal,
 * expected outcome of clicking twice, and must not render as an error.
 *
 * A 429 without a parsable `Retry-After` still counts as a throttle: parking
 * for the standard cooldown is closer to the truth than treating it as a
 * failure the user should retry immediately. It comes back marked
 * `approximate` so the copy can say roughly how long rather than pretend to a
 * precision it does not have.
 */
export function verificationResendThrottle(
  error: unknown
): VerificationResendThrottle | null {
  if (!error || typeof error !== 'object') {
    return null
  }
  const apiError = error as ApiError
  if (apiError.status !== 429) {
    return null
  }
  const retryAfter = apiError.retryAfter
  if (typeof retryAfter === 'number' && Number.isFinite(retryAfter) && retryAfter > 0) {
    return { seconds: Math.ceil(retryAfter), precision: 'exact' }
  }
  return {
    seconds: VERIFICATION_RESEND_COOLDOWN_SECONDS,
    precision: 'approximate',
  }
}

/**
 * How much room the surface has for the status line.
 *
 * `default` is the landing-surface phrasing; `compact` is the settings-row
 * phrasing, which sits in a dense column beside the button rather than on its
 * own line.
 */
export type ResendStatusDensity = 'default' | 'compact'

interface ResendStatusInput {
  sent: boolean
  secondsRemaining: number
  precision: ResendWaitPrecision
  density?: ResendStatusDensity
}

/**
 * The mono status line under a resend control, or `null` when there is nothing
 * to say yet.
 *
 * Both halves are independent: a user can be waiting without having sent
 * anything from this surface (they were throttled on the first click), and a
 * send can be confirmed after the wait has run out.
 *
 * An `approximate` wait deliberately drops the second count. Rendering
 * "available in 59s" off a number the app guessed would be a precise-looking
 * lie, and it would tick down convincingly for a whole minute.
 */
export function formatResendStatus({
  sent,
  secondsRemaining,
  precision,
  density = 'default',
}: ResendStatusInput): string | null {
  const compact = density === 'compact'
  let wait: string | null = null
  if (secondsRemaining > 0) {
    if (precision === 'exact') {
      wait = compact
        ? `Again in ${secondsRemaining}s`
        : `Resend available in ${secondsRemaining}s`
    } else {
      // "about a minute" is safe to say because the only producer of an
      // approximate wait is `verificationResendThrottle`, which always parks
      // for VERIFICATION_RESEND_COOLDOWN_SECONDS — itself matched to the
      // backend's `Retry-After: 60`. Change one and this sentence changes.
      wait = compact
        ? 'Again in about a minute'
        : 'Resend available in about a minute'
    }
  }
  const confirmation = sent ? (compact ? 'Sent' : 'Sent · Check your inbox') : null
  const parts = [confirmation, wait].filter(Boolean)
  return parts.length > 0 ? parts.join(' · ') : null
}

/**
 * True when a resend failed because the session is gone rather than because
 * the send itself broke.
 *
 * Worth separating for two reasons: the reader needs a way forward (sign in
 * again), not a generic "try later"; and a cookie expiring while a card sat
 * open is an ordinary event, so it must not page on-call as a Sentry error.
 */
export function isVerificationResendUnauthorized(error: unknown): boolean {
  if (!error || typeof error !== 'object') {
    return false
  }
  const status = (error as ApiError).status
  return status === 401 || status === 403
}

/**
 * What a screen reader should hear, or `null` when there is nothing to say.
 *
 * Deliberately carries no second count and does NOT vary with the cooldown.
 * The visible line ticks once a second, and a polite live region whose text
 * changes that often announces sixty times over one cooldown. Keying the
 * announcement off `sent` alone also means the region stays byte-identical
 * when the wait runs out, so nobody gets "Verification email sent" read back
 * to them a minute after they sent it, unprompted.
 */
export function resendStatusAnnouncement(
  sent: boolean,
  isCoolingDown: boolean
): string | null {
  if (sent) {
    return 'Verification email sent. Check your inbox.'
  }
  if (isCoolingDown) {
    return 'Resend is not available yet. Please wait a moment.'
  }
  return null
}

interface VerificationResendCooldown {
  /** Whole seconds left before the control is usable again; 0 when idle. */
  secondsRemaining: number
  isCoolingDown: boolean
  /**
   * Whether `secondsRemaining` is a real deadline or a local assumption. The
   * button is disabled for the same span either way; only the copy differs.
   */
  precision: ResendWaitPrecision
  /**
   * Parks the control for `seconds`; a later call replaces an active wait.
   * Defaults to `exact` because the caller that omits it is the successful-send
   * path, where this module itself owns the deadline.
   */
  start: (seconds: number, precision?: ResendWaitPrecision) => void
}

/**
 * Countdown state for a verification-resend control.
 *
 * Deliberately owns only the wait, not the mutation. Its one consumer is the
 * shared `<VerificationResend>` control, which pairs it with the mutation;
 * keeping the two apart lets the countdown be unit-tested on its own, and means
 * a suite that mocks the `@/features/auth` barrel still exercises it for real,
 * since it is imported by module path rather than through that barrel.
 *
 * The countdown is driven off an absolute deadline rather than a decrementing
 * counter, so a backgrounded tab that stops firing timers resumes with the
 * correct remaining time instead of a stale one.
 *
 * The interval lives in an effect and is cleared on unmount. A timer left
 * running past unmount fires `setState` into a torn-down jsdom and fails the
 * whole vitest run (see the PSY-1664 note on `useDismissTimer`); the shared
 * one-shot primitives there do not fit a repeating countdown, so this is an
 * effect-scoped interval with explicit cleanup rather than a bare one.
 */
export function useVerificationResendCooldown(): VerificationResendCooldown {
  const [deadline, setDeadline] = useState<number | null>(null)
  const [secondsRemaining, setSecondsRemaining] = useState(0)
  const [precision, setPrecision] = useState<ResendWaitPrecision>('exact')

  const start = useCallback(
    (seconds: number, nextPrecision: ResendWaitPrecision = 'exact') => {
      if (!Number.isFinite(seconds) || seconds <= 0) {
        return
      }
      setSecondsRemaining(Math.ceil(seconds))
      setPrecision(nextPrecision)
      setDeadline(Date.now() + Math.ceil(seconds) * 1000)
    },
    []
  )

  useEffect(() => {
    if (deadline === null) {
      return
    }

    const tick = () => {
      const remaining = Math.ceil((deadline - Date.now()) / 1000)
      if (remaining <= 0) {
        setSecondsRemaining(0)
        setDeadline(null)
        return
      }
      setSecondsRemaining(remaining)
    }

    const intervalId = setInterval(tick, 1000)
    return () => clearInterval(intervalId)
  }, [deadline])

  return {
    secondsRemaining,
    isCoolingDown: secondsRemaining > 0,
    precision,
    start,
  }
}

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
 * Reads the wait a throttled resend asks for, in whole seconds.
 *
 * Returns `null` for anything that is not a throttle, so callers can keep the
 * "please wait" state and the "that failed" state apart: a 429 is a normal,
 * expected outcome of clicking twice, and must not render as an error.
 *
 * A 429 without a parsable `Retry-After` still counts as a throttle: falling
 * back to the standard cooldown is closer to the truth than treating it as a
 * failure the user should retry immediately.
 */
export function verificationResendRetryAfter(error: unknown): number | null {
  if (!error || typeof error !== 'object') {
    return null
  }
  const apiError = error as ApiError
  if (apiError.status !== 429) {
    return null
  }
  const retryAfter = apiError.retryAfter
  if (typeof retryAfter === 'number' && Number.isFinite(retryAfter) && retryAfter > 0) {
    return Math.ceil(retryAfter)
  }
  return VERIFICATION_RESEND_COOLDOWN_SECONDS
}

/**
 * The mono status line under a resend control, or `null` when there is nothing
 * to say yet.
 *
 * Both halves are independent: a user can be waiting without having sent
 * anything from this surface (they were throttled on the first click), and a
 * send can be confirmed after the wait has run out.
 */
export function formatResendStatus(
  sent: boolean,
  secondsRemaining: number
): string | null {
  const wait =
    secondsRemaining > 0 ? `Resend available in ${secondsRemaining}s` : null
  const confirmation = sent ? 'Sent · Check your inbox' : null
  const parts = [confirmation, wait].filter(Boolean)
  return parts.length > 0 ? parts.join(' · ') : null
}

/**
 * What a screen reader should hear, or `null` when there is nothing to say.
 *
 * Deliberately carries no second count. The visible line ticks once a second,
 * and a polite live region whose text changes that often announces sixty times
 * over one cooldown, burying the one fact that matters. This string changes
 * only when the state does, so call sites render it inside the live region and
 * mark the ticking copy `aria-hidden`.
 */
export function resendStatusAnnouncement(
  sent: boolean,
  isCoolingDown: boolean
): string | null {
  if (sent) {
    return isCoolingDown
      ? 'Verification email sent. Check your inbox. You can send another in about a minute.'
      : 'Verification email sent. Check your inbox.'
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
  /** Parks the control for `seconds`; a later call replaces an active wait. */
  start: (seconds: number) => void
}

/**
 * Countdown state for a verification-resend control.
 *
 * Deliberately owns only the wait, not the mutation: the surfaces that use it
 * (the /verify-email expired card, the /shows/submit gate, the Settings account
 * row) render the wait differently and already hold the mutation themselves.
 * Keeping the mutation out also means a test that mocks the `@/features/auth`
 * barrel still exercises this countdown for real, since it is imported by
 * module path rather than through that barrel.
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

  const start = useCallback((seconds: number) => {
    if (!Number.isFinite(seconds) || seconds <= 0) {
      return
    }
    setSecondsRemaining(Math.ceil(seconds))
    setDeadline(Date.now() + Math.ceil(seconds) * 1000)
  }, [])

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
    start,
  }
}

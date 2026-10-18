import { useCallback, useEffect, useMemo, useRef } from 'react'
import { useDismissTimer } from '@/lib/hooks/common/useDismissTimer'

/** How long a sign-up request may go unanswered before it is abandoned. */
export const SIGNUP_REQUEST_TIMEOUT_MS = 30_000

/**
 * A deadline for one sign-up request at a time. Requests that may be in flight
 * together each need a deadline of their own.
 *
 * `arm()` returns the signal to hand the request. Once
 * SIGNUP_REQUEST_TIMEOUT_MS passes without `disarm()`, the signal aborts and
 * then `onExpire` runs. Arming again replaces the previous deadline. The signal
 * covers the whole response, body included, so a response that stalls after
 * its headers is abandoned too.
 *
 * A request identifies itself by the signal it was armed with, so one that
 * outlives the control that sent it cannot touch a newer request's deadline:
 * `rearm` and a `disarm` given a signal act only while that signal is the one
 * armed.
 */
export interface SignupRequestDeadline {
  arm: () => AbortSignal
  /**
   * Restarts the deadline of the request armed with `signal` and returns the
   * signal that now stands for it. Any other signal comes back unchanged and
   * the deadline is left alone.
   */
  rearm: (signal: AbortSignal) => AbortSignal
  disarm: (signal?: AbortSignal) => void
}

/**
 * The page that holds the SignupHandoff owns this deadline, not the control
 * that sends the request: the control unmounts whenever the page stops
 * rendering the form, a pending claim included, and the deadline has to outlive
 * it to release that claim. Unmounting the owner cancels the deadline without
 * aborting the request.
 *
 * Hiding the owner cancels the timer the same way, since a hidden page runs its
 * effect cleanups. Showing it again gives a request still under the deadline a
 * fresh SIGNUP_REQUEST_TIMEOUT_MS, on the signal that request already holds.
 */
export function useSignupRequestDeadline(
  onExpire: () => void
): SignupRequestDeadline {
  // The controller of the request under the deadline: set by arm(), cleared by
  // disarm() and by the expiry.
  const controllerRef = useRef<AbortController | null>(null)
  const { schedule, cancel } = useDismissTimer(() => {
    const controller = controllerRef.current
    controllerRef.current = null
    controller?.abort()
    onExpire()
  }, SIGNUP_REQUEST_TIMEOUT_MS)

  const arm = useCallback(() => {
    const controller = new AbortController()
    controllerRef.current = controller
    schedule()
    return controller.signal
  }, [schedule])

  const rearm = useCallback(
    (signal: AbortSignal) =>
      controllerRef.current?.signal === signal ? arm() : signal,
    [arm]
  )

  const disarm = useCallback(
    (signal?: AbortSignal) => {
      if (signal && controllerRef.current?.signal !== signal) {
        return
      }
      controllerRef.current = null
      cancel()
    },
    [cancel]
  )

  useEffect(() => {
    if (controllerRef.current !== null) {
      schedule()
    }
  }, [schedule])

  return useMemo(() => ({ arm, rearm, disarm }), [arm, rearm, disarm])
}

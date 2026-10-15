import { useCallback, useMemo, useRef } from 'react'
import { useDismissTimer } from '@/lib/hooks/common/useDismissTimer'

/** How long a sign-up request may go unanswered before it is abandoned. */
export const SIGNUP_REQUEST_TIMEOUT_MS = 30_000

/**
 * A deadline for the sign-up request in flight, one request at a time.
 *
 * `arm()` returns the signal to hand the request. Once
 * SIGNUP_REQUEST_TIMEOUT_MS passes without `disarm()`, the signal aborts and
 * then `onExpire` runs. Arming again replaces the previous deadline. The signal
 * covers the whole response, body included, so a response that stalls after
 * its headers is abandoned too.
 */
export interface SignupRequestDeadline {
  arm: () => AbortSignal
  disarm: () => void
}

/**
 * The page that holds the SignupHandoff owns this deadline, not the control
 * that sends the request: the control unmounts whenever the page stops
 * rendering the form, a pending claim included, and the deadline has to outlive
 * it to release that claim. Unmounting the owner cancels the deadline without
 * aborting the request.
 */
export function useSignupRequestDeadline(
  onExpire: () => void
): SignupRequestDeadline {
  // Read only by the timer, which runs only while the latest arm() is pending.
  const controllerRef = useRef<AbortController | null>(null)
  const { schedule, cancel } = useDismissTimer(() => {
    controllerRef.current?.abort()
    onExpire()
  }, SIGNUP_REQUEST_TIMEOUT_MS)

  const arm = useCallback(() => {
    const controller = new AbortController()
    controllerRef.current = controller
    schedule()
    return controller.signal
  }, [schedule])

  return useMemo(() => ({ arm, disarm: cancel }), [arm, cancel])
}

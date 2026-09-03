'use client'

import { useCallback } from 'react'
import { usePathname, useRouter } from 'next/navigation'
import { useAuthContext } from '@/lib/context/AuthContext'
import type { AuthStatus } from '@/lib/context/AuthContext'
import { buildAuthHref, currentLocationReturnTo } from '@/lib/auth-href'

/**
 * The two calls a caller may still make for itself.
 *
 * `onAnonymous` exists for a control that surfaces sign-in as a dialog rather
 * than a navigation; it receives the same href the default push would use, so
 * the destination stays one formula no matter which shape the affordance
 * takes.
 */
export interface AuthGatedActionOptions {
  onAnonymous?: (authHref: string) => void
}

export interface AuthGatedAction {
  authStatus: AuthStatus
  /** Auth is unsettled. The control renders disabled and cannot act. */
  isPending: boolean
  /** Settled anonymous: a click routes to sign-in rather than acting. */
  isAnonymous: boolean
  isAuthenticated: boolean
  /**
   * The sign-in href for the viewer's current location. Event-time only, for
   * the reason `currentLocationReturnTo` gives.
   */
  buildAuthHrefForHere: () => string
  /** The gated handler. Assignable straight to `onClick`. */
  onClick: (event?: {
    preventDefault: () => void
    stopPropagation: () => void
  }) => void
}

/**
 * One owner for the three-state branch every auth-gated control runs:
 *
 *   pending       do nothing; the viewer's identity is not known, and the
 *                 sign-in redirect cannot tell "no session" from "profile in
 *                 flight", so acting on it sends a signed-in viewer to the
 *                 sign-in form
 *   anonymous     route to `/auth` with the canonical returnTo
 *   authenticated run `action`
 *
 * `isPending` is exposed so the caller can render the control disabled in the
 * same window the handler refuses to act in. A control that only guards the
 * click still renders actionable and is silently inert, which is a different
 * bug from the one this hook closes.
 *
 * The handler suppresses the event's default and propagation before it
 * branches, because every control in this class sits inside a linkbox or a
 * card that would otherwise navigate underneath it.
 */
export function useAuthGatedAction(
  action: () => void,
  options?: AuthGatedActionOptions
): AuthGatedAction {
  const router = useRouter()
  const pathname = usePathname()
  const { authStatus } = useAuthContext()
  const onAnonymous = options?.onAnonymous

  const buildAuthHrefForHere = useCallback(
    () => buildAuthHref(currentLocationReturnTo(pathname)),
    [pathname]
  )

  const onClick = useCallback(
    (event?: { preventDefault: () => void; stopPropagation: () => void }) => {
      event?.preventDefault()
      event?.stopPropagation()

      if (authStatus === 'pending') return

      if (authStatus === 'anonymous') {
        const href = buildAuthHrefForHere()
        if (onAnonymous) {
          onAnonymous(href)
        } else {
          router.push(href)
        }
        return
      }

      action()
    },
    [action, authStatus, buildAuthHrefForHere, onAnonymous, router]
  )

  return {
    authStatus,
    isPending: authStatus === 'pending',
    isAnonymous: authStatus === 'anonymous',
    isAuthenticated: authStatus === 'authenticated',
    buildAuthHrefForHere,
    onClick,
  }
}

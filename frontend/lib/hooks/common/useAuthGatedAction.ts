'use client'

import { useCallback } from 'react'
import { usePathname, useRouter } from 'next/navigation'
import { useAuthContext } from '@/lib/context/AuthContext'
import {
  buildAuthHref,
  buildGatedAuthHref,
  currentLocationReturnTo,
  type AuthIntent,
} from '@/lib/auth-href'

export interface AuthGatedAction {
  /**
   * The sign-in href for the viewer's current location, naming no intent, so
   * it opens on Sign in. Event-time only, for the reason
   * `currentLocationReturnTo` gives.
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
 *   anonymous     route to `/auth` with the canonical returnTo and `intent`,
 *                 which opens the page on Create account
 *   authenticated run `action`
 *
 * The handler is half the rule. A control that only guards the click still
 * renders actionable and is silently inert, so the caller must also render it
 * disabled while `authStatus === 'pending'`.
 *
 * The handler suppresses the event's default and propagation before it
 * branches, because every control in this class sits inside a linkbox or a
 * card that would otherwise navigate underneath it.
 *
 * `onAnonymous` is for a control that surfaces sign-in as a dialog rather than
 * a navigation. It receives `buildAuthHrefForHere()`: the same returnTo the
 * default push carries, without the intent, because a dialog offers Sign in
 * and Create account as separate choices and the intent would open Create
 * account under the one labelled Sign in.
 */
export function useAuthGatedAction(
  intent: AuthIntent,
  action: () => void,
  onAnonymous?: (authHref: string) => void
): AuthGatedAction {
  const router = useRouter()
  const pathname = usePathname()
  const { authStatus } = useAuthContext()

  const buildAuthHrefForHere = useCallback(
    () => buildAuthHref(currentLocationReturnTo(pathname)),
    [pathname]
  )

  // Deliberately not memoized: every caller passes a fresh inline `action`, so
  // a `useCallback` here could never hit its cache, and nothing downstream is
  // memoized on this identity.
  const onClick = (event?: {
    preventDefault: () => void
    stopPropagation: () => void
  }) => {
    event?.preventDefault()
    event?.stopPropagation()

    if (authStatus === 'pending') return

    if (authStatus === 'anonymous') {
      if (onAnonymous) {
        onAnonymous(buildAuthHrefForHere())
      } else {
        router.push(
          buildGatedAuthHref(currentLocationReturnTo(pathname), intent)
        )
      }
      return
    }

    action()
  }

  return { buildAuthHrefForHere, onClick }
}

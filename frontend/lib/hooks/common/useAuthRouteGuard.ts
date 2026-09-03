'use client'

import { useEffect } from 'react'
import { redirect, usePathname, useRouter } from 'next/navigation'
import { useAuthContext } from '@/lib/context/AuthContext'
import { buildAuthHref, currentLocationReturnTo } from '@/lib/auth-href'

/**
 * How a guard leaves the page for `/auth`.
 *
 * Three values because the guarded pages already navigate three ways, and the
 * mechanism is not this hook's decision to change: `redirect` throws during
 * render and so also answers a server render, while the router modes navigate
 * from an effect and differ only in what they leave in history.
 */
export type AuthGuardNavigation = 'push' | 'replace' | 'redirect'

/**
 * What the guarded page should render.
 *
 *   'loading' auth is unsettled; render the page's own loading state
 *   'blank'   the sign-in navigation is issued; render nothing
 *   'ready'   render the page
 */
export type AuthGuardRender = 'loading' | 'blank' | 'ready'

/**
 * Route guard for a page that requires a signed-in viewer.
 *
 * The rule it owns, and the reason it is one hook rather than a dozen copies:
 * a guard may redirect only on a SETTLED anonymous answer. While
 * `authStatus === 'pending'` the viewer's identity is unknown, and treating
 * unknown as anonymous bounces a signed-in viewer to the sign-in form and
 * loses the page they were on. `isLoading` cannot express that rule: it is
 * false both before the profile fetch starts and after it fails without
 * settling. See {@link import('@/lib/context/AuthContext').AuthStatus}.
 *
 * Admin and email-verification checks layer on top of a `'ready'` verdict;
 * they are about what the viewer may do, not about whether the viewer is
 * known, and they must not run before this answers `'ready'`.
 */
export function useAuthRouteGuard(
  navigation: AuthGuardNavigation = 'push'
): AuthGuardRender {
  const router = useRouter()
  const pathname = usePathname()
  const { authStatus } = useAuthContext()

  useEffect(() => {
    if (authStatus !== 'anonymous') return
    if (navigation === 'redirect') return
    const href = buildAuthHref(currentLocationReturnTo(pathname))
    if (navigation === 'replace') {
      router.replace(href)
    } else {
      router.push(href)
    }
  }, [authStatus, navigation, pathname, router])

  if (authStatus === 'pending') return 'loading'

  if (authStatus === 'anonymous') {
    // Throws, so the caller never sees 'blank' in this mode. It also throws
    // BEFORE any hook the caller declares after this one, which is safe only
    // because React discards a render that throws: there is no partial commit
    // to leave a hook list short.
    if (navigation === 'redirect') {
      redirect(buildAuthHref(currentLocationReturnTo(pathname)))
    }
    return 'blank'
  }

  return 'ready'
}

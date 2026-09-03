/**
 * Builds links INTO the auth page. The inverse, which reads `returnTo` back
 * out and decides whether to trust it, is `sanitizeReturnTo` in
 * `app/auth/auth-redirect-utils.ts` — change one and check the other.
 *
 * This lives in `lib/` rather than beside its inverse because the consumers
 * span `app/` and `features/` both, and route-local modules under `app/auth/`
 * should not become shared dependencies of the feature layer.
 */

/** The route that renders the sign-in and create-account forms. */
export const AUTH_PATH = '/auth'

/** Where a rejected or absent `returnTo` lands. */
export const FALLBACK_RETURN_TO = '/'

/**
 * Is this path part of the sign-in surface itself?
 *
 * Lives here, with the builder, and is imported by `sanitizeReturnTo` rather
 * than restated there: both halves of the contract have to agree about which
 * destinations the auth page refuses, and a second copy is free to drift.
 */
export function isAuthPath(pathname: string): boolean {
  return pathname === AUTH_PATH || pathname.startsWith(`${AUTH_PATH}/`)
}

/**
 * Builds an auth-page href that sends the reader back to `returnTo` once they
 * are signed in.
 *
 * `returnTo` is encoded whole, so a destination carrying its own query string
 * or fragment survives the round trip instead of being parsed as part of the
 * auth page's own params.
 *
 * A destination `sanitizeReturnTo` would discard is omitted rather than
 * encoded: the auth surface's own routes and the fallback both land the reader
 * at `/` with or without the param, so emitting one would put a self-defeating
 * query string in the markup of every page that carries no destination worth
 * returning to.
 */
export function buildAuthHref(returnTo: string): string {
  const path = returnTo.split(/[?#]/)[0]
  if (returnTo === FALLBACK_RETURN_TO || isAuthPath(path)) return AUTH_PATH
  return `${AUTH_PATH}?returnTo=${encodeURIComponent(returnTo)}`
}

/**
 * The destination a viewer sent to sign-in comes back to: the path they are
 * on plus its query string, so a filtered list returns filtered.
 *
 * This is the only spelling of that formula. `useAuthGatedAction` and
 * `useAuthRouteGuard` both build their href from it, so a control and a guard
 * on the same page cannot disagree about where the reader came from.
 *
 * The query string is read from `window.location.search` rather than
 * `useSearchParams`, which forces a Suspense boundary on every consumer and
 * would opt entity pages out of static rendering. That makes this correct
 * only where a browser location exists: call it from an event handler or an
 * effect, never during render. Without one it yields the bare pathname, which
 * is why a render-time sign-in href is built from the pathname alone instead
 * (`features/auth/components/SignInPrompt.tsx` states that constraint).
 *
 * That render-time grade has three call sites and no function of its own,
 * because it is just the pathname: `SignInPrompt`, `nav/UserMenu`'s
 * `SignInLink`, and `nav/BottomTabBar`'s Account cell. A fourth belongs beside
 * them, not as a fresh formula.
 */
export function currentLocationReturnTo(pathname: string): string {
  const search = typeof window === 'undefined' ? '' : window.location.search
  return `${pathname}${search}`
}

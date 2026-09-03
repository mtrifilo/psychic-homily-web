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

/**
 * Builds an auth-page href that sends the reader back to `returnTo` once they
 * are signed in.
 *
 * `returnTo` is encoded whole, so a destination carrying its own query string
 * or fragment survives the round trip instead of being parsed as part of the
 * auth page's own params.
 */
export function buildAuthHref(returnTo: string): string {
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
 */
export function currentLocationReturnTo(pathname: string): string {
  const search = typeof window === 'undefined' ? '' : window.location.search
  return `${pathname}${search}`
}

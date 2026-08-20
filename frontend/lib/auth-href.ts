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

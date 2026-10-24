// Direct backend origin, never the Next.js /api proxy: OAuth is a full-page
// redirect and the redirect chain does not survive the proxy. See
// lib/api-base.ts for the resolution order.
import { OAUTH_BACKEND_URL } from '@/lib/api-base'
import { FALLBACK_RETURN_TO } from '@/lib/auth-href'

/**
 * The backend route that starts a Google sign-in. The same start serves a new
 * account and a returning one: a new Google identity finishes on the "Create
 * your account" step (/auth/complete), where its consent is asked for.
 *
 * The route reads `returnTo` (`oauthReturnToParam` in
 * backend/internal/api/handlers/auth/oauth_return_to.go), checks it again,
 * records it for its callback, and lands on the site root for anything it
 * refuses. `selectAccount` asks Google for its account chooser, for "Not you?
 * Use a different account" on the step.
 */
export function googleStartUrl({
  returnTo,
  selectAccount = false,
}: {
  returnTo: string
  selectAccount?: boolean
}): string {
  const url = new URL(`${OAUTH_BACKEND_URL}/auth/login/google`)
  if (returnTo !== FALLBACK_RETURN_TO) {
    url.searchParams.set('returnTo', returnTo)
  }
  if (selectAccount) {
    url.searchParams.set('prompt', 'select_account')
  }
  return url.toString()
}

// Direct backend origin, never the Next.js /api proxy: OAuth is a full-page
// redirect and the redirect chain does not survive the proxy. See
// lib/api-base.ts for the resolution order.
import { OAUTH_BACKEND_URL } from '@/lib/api-base'
import {
  AUTH_REASON_PARAM,
  FALLBACK_RETURN_TO,
  type ReauthReason,
} from '@/lib/auth-href'

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
 *
 * `reauthReason` is the re-authentication the auth page is showing, when it
 * shows one. The route records it beside `returnTo`, and a refused callback
 * hands it back with the refusal's code, so the page reopens as the same
 * re-authentication with the refusal shown instead of sending a signed-in
 * reader on to `returnTo`. A sign-in that succeeds drops it.
 */
export function googleStartUrl({
  returnTo,
  selectAccount = false,
  reauthReason,
}: {
  returnTo: string
  selectAccount?: boolean
  reauthReason?: ReauthReason
}): string {
  const url = new URL(`${OAUTH_BACKEND_URL}/auth/login/google`)
  if (returnTo !== FALLBACK_RETURN_TO) {
    url.searchParams.set('returnTo', returnTo)
  }
  if (reauthReason !== undefined) {
    url.searchParams.set(AUTH_REASON_PARAM, reauthReason)
  }
  if (selectAccount) {
    url.searchParams.set('prompt', 'select_account')
  }
  return url.toString()
}

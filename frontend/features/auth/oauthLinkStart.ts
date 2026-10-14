// Direct backend origin, never the Next.js /api proxy: connecting an account
// is a full-page OAuth redirect. See lib/api-base.ts.
import { OAUTH_BACKEND_URL } from '@/lib/api-base'

/**
 * The URL that starts connecting a provider to the SIGNED-IN account, given a
 * one-time token from useStartOAuthLink.
 *
 * The authenticated link route, never /auth/login/{provider}: the link route
 * takes the account from the session, while the sign-in route takes it from
 * what the provider returns.
 *
 * The token is minted by a same-origin authenticated request first. The link
 * route is a plain GET and the auth cookie is SameSite=Lax, which a browser
 * sends on a cross-site top-level navigation, so without a token only our own
 * page can hold, any site could push a signed-in user through the connect
 * flow.
 */
export function oauthLinkStartUrl(provider: 'google', token: string): string {
  return `${OAUTH_BACKEND_URL}/auth/link/${provider}?t=${encodeURIComponent(token)}`
}

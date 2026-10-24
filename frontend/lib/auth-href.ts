/**
 * Builds links INTO the auth page. The inverse, which reads `returnTo` back
 * out and decides whether to trust it, is `sanitizeReturnTo` in
 * `app/auth/auth-redirect-utils.ts` — change one and check the other.
 *
 * This lives in `lib/` rather than beside its inverse because the consumers
 * span `app/` and `features/` both, and route-local modules under `app/auth/`
 * should not become shared dependencies of the feature layer.
 */

import { AuthErrorCode } from '@/lib/errors/authErrors'

/** The route that renders the sign-in and create-account forms. */
export const AUTH_PATH = '/auth'

/**
 * Where a rejected or absent `returnTo` lands. The backend's Google sign-in
 * falls back to the same path (`oauthReturnToFallback` in
 * `backend/internal/api/handlers/auth/oauth_return_to.go`).
 */
export const FALLBACK_RETURN_TO = '/'

/**
 * Is this path part of the sign-in surface itself?
 *
 * Lives here, with the builder, and is imported by `sanitizeReturnTo` rather
 * than restated there: both halves of the contract have to agree about which
 * destinations the auth page refuses, and a second copy is free to drift.
 *
 * Go cannot import it, so `sanitizeOAuthReturnTo` in
 * `backend/internal/api/handlers/auth/oauth_return_to.go` restates the rule
 * for Google sign-in; a change here has to be made there too. Both sanitizers
 * judge the percent-decoded path, so "/%61uth" is the auth page to them. The
 * builder judges the path as written, so it can carry such a spelling, which
 * the auth page then refuses: the reader still lands on FALLBACK_RETURN_TO.
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
 * The fallback and the auth surface's own routes are omitted rather than
 * encoded: `sanitizeReturnTo` discards them, and they land the reader at `/`
 * with or without the param, so emitting one would put a self-defeating
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
 * That render-time grade has four call sites and no function of its own,
 * because it is just the pathname: `SignInPrompt`, `nav/UserMenu`'s
 * `SignInLink`, `nav/BottomTabBar`'s signed-out Account sheet, and
 * `components/shared/CredentialErrorMessage`. A fifth belongs beside them, not
 * as a fresh formula.
 *
 * An OAuth link result is left out of the query. Settings reads it once, on
 * the redirect that delivers it; kept in returnTo it would replay after any
 * later sign-in through this link, however long after it stopped being true.
 * `sanitizeReturnTo` and the backend's `sanitizeOAuthReturnTo` drop it again
 * when they read returnTo, so a link built elsewhere cannot replay it either;
 * leaving it out here keeps it out of the auth page's own URL.
 */
export function currentLocationReturnTo(pathname: string): string {
  const search = typeof window === 'undefined' ? '' : window.location.search
  return `${pathname}${withoutOAuthLinkResult(search)}`
}

/**
 * The query parameters the backend's OAuth link redirect delivers its result
 * under, read by the Settings panel. The backend writes them as
 * `oauthLinkResultParam` and `oauthLinkErrorParam` in
 * `backend/internal/api/handlers/auth/oauth_link.go`; the names on both sides
 * must stay the same.
 */
export const OAUTH_LINK_RESULT_PARAM = 'oauth_link'
export const OAUTH_LINK_ERROR_PARAM = 'oauth_link_error'

const OAUTH_LINK_RESULT_PARAMS: readonly string[] = [
  OAUTH_LINK_RESULT_PARAM,
  OAUTH_LINK_ERROR_PARAM,
]

/**
 * `search` without the OAuth link result parameters. Every other parameter
 * keeps the spelling the location gave it, so a destination that carries no
 * link result comes back byte for byte. A name is compared decoded, the way
 * Settings reads it, so an encoded spelling of a link parameter is dropped
 * too. Returns `''` or a query string with its leading `?`; the leading `?` on
 * the input is optional.
 */
export function withoutOAuthLinkResult(search: string): string {
  const kept = search
    .replace(/^\?/, '')
    .split('&')
    .filter(pair => pair !== '' && !isOAuthLinkResultPair(pair))
  return kept.length === 0 ? '' : `?${kept.join('&')}`
}

/**
 * Is this one `name=value` pair of a query string an OAuth link result? The
 * name is everything before the first `=`, decoded the way a URL parser
 * decodes a query (`+` is a space), and a name that does not decode names
 * neither parameter. A pair split out of a query string is never stripped of
 * a leading `?`: in `??oauth_link=x` the name is `?oauth_link`.
 */
export function isOAuthLinkResultPair(pair: string): boolean {
  const [name] = pair.split('=', 1)
  try {
    return OAUTH_LINK_RESULT_PARAMS.includes(
      decodeURIComponent(name.replace(/\+/g, ' '))
    )
  } catch {
    return false
  }
}

/**
 * The `reason` the backend puts on an `/auth` link when it is asking an
 * already-signed-in reader to prove the account again before a
 * security-relevant change, rather than asking a stranger to sign in.
 *
 * Written by redirectToReauth in
 * backend/internal/api/handlers/auth/oauth_link.go. The auth page reads it to
 * suppress its authenticated-reader bounce, which would otherwise return them
 * to the control that just refused them with nothing changed.
 */
export const REAUTH_REASON_OAUTH_LINK = 'OAUTH_LINK_REAUTH_REQUIRED'

/**
 * The reason a credential mint refuses with. It is the backend error code
 * verbatim (autherrors.CodeReauthRequired), so the API refusal and the link the
 * surface offers name the same thing rather than two spellings of it.
 *
 * The auth page reads it exactly as it reads REAUTH_REASON_OAUTH_LINK: to
 * suppress its authenticated-reader bounce, which would otherwise return the
 * reader to the control that just refused them with nothing changed.
 */
export const REAUTH_REASON_CREDENTIAL_MINT = 'REAUTH_REQUIRED'

/** Every reason that means "an already-signed-in reader must prove the account". */
export const REAUTH_REASONS: readonly string[] = [
  REAUTH_REASON_OAUTH_LINK,
  REAUTH_REASON_CREDENTIAL_MINT,
]

export function isReauthReason(reason: string | null): boolean {
  return reason !== null && REAUTH_REASONS.includes(reason)
}

/**
 * The reason redirectToAccountUnavailable
 * (backend/internal/api/handlers/auth/oauth_link.go) puts on an `/auth` link:
 * an OAuth link refused because the account may no longer sign in. Not a
 * re-authentication reason, since signing in again cannot satisfy it.
 */
export const ACCOUNT_INACTIVE_REASON = AuthErrorCode.ACCOUNT_INACTIVE

/**
 * The auth-page query parameter that says why the reader was sent there. The
 * backend writes it as `authPageReasonParam` in
 * `backend/internal/api/handlers/auth/oauth_handlers.go`.
 */
export const AUTH_REASON_PARAM = 'reason'

/**
 * The auth-page href for a reader who is signed in and has to prove it again.
 *
 * Carries the reason so the page shows them the form instead of bouncing them,
 * and returnTo so completing a factor puts them back on the control that
 * refused them. Built on buildAuthHref, so a destination that sanitizeReturnTo
 * would discard is dropped the same way it is everywhere else.
 */
export function buildReauthHref(returnTo: string): string {
  return appendAuthParam(
    buildAuthHref(returnTo),
    AUTH_REASON_PARAM,
    REAUTH_REASON_CREDENTIAL_MINT
  )
}

/**
 * The query parameter that names the kind of action an anonymous viewer tried
 * to take when a gated control turned them away.
 *
 * Unexported: `buildGatedAuthHref` writes it and `hasGatedIntent` reads it, so
 * no other module can import it, and `authGateOwnership.test.ts` refuses the
 * parameter spelled out anywhere else. The auth page opens on Create account when it
 * carries a listed value, unless the arrival is a re-authentication. Every
 * other route into the page (the header link, the mobile Account sheet, a route
 * guard, `SignInPrompt`, a re-auth link) names no intent and opens on Sign in,
 * except the home hero's sign-up nudge, which asks for Create account through
 * `AUTH_TAB_PARAM` instead.
 *
 * The value names a kind of action, never its target, so it is enough to
 * choose a tab and not enough to repeat the action.
 */
const AUTH_INTENT_PARAM = 'intent'

/**
 * Every action a gated control can name, and so every value the auth page
 * accepts. Anything outside this list is ignored, so the parameter carries
 * nothing the page does not already expect.
 */
export const AUTH_INTENTS = [
  'save',
  'follow',
  'collect',
  'like',
  'notify',
  'confirm',
  'report',
] as const

export type AuthIntent = (typeof AUTH_INTENTS)[number]

/** Does this auth-page URL name an intent a gated control can set? */
export function hasGatedIntent(searchParams: {
  get: (name: string) => string | null
}): boolean {
  const raw = searchParams.get(AUTH_INTENT_PARAM)
  return raw !== null && (AUTH_INTENTS as readonly string[]).includes(raw)
}

/**
 * The query parameter that asks the auth page to open on Create account for a
 * reader who followed a link labelled as signing up, and the one value it
 * accepts.
 *
 * Unexported: `buildSignupTabAuthHref` writes it and `hasSignupTab` reads it,
 * and `authGateOwnership.test.ts` pins the home hero's sign-up nudge as the
 * only module outside this one that builds it. Like `intent`, it chooses a
 * tab and nothing else, and a re-authentication still opens on Sign in.
 */
const AUTH_TAB_PARAM = 'tab'
const SIGNUP_TAB = 'signup'

/**
 * The auth-page href for the home hero's sign-up nudge: Create account, with
 * no `returnTo`, because the hero renders only on `/` and `sanitizeReturnTo`
 * discards a returnTo naming the site root.
 */
export function buildSignupTabAuthHref(): string {
  return appendAuthParam(AUTH_PATH, AUTH_TAB_PARAM, SIGNUP_TAB)
}

/**
 * Does this auth-page URL ask for the Create account tab? Only the exact
 * value `signup` does; any other value, or none, is ignored.
 */
export function hasSignupTab(searchParams: {
  get: (name: string) => string | null
}): boolean {
  return searchParams.get(AUTH_TAB_PARAM) === SIGNUP_TAB
}

/**
 * The auth-page query parameters a failed OAuth sign-in callback writes: the
 * message the page shows, and the code of a refusal the reader can act on.
 * The backend writes them as `authPageErrorParam` and `authPageErrorCodeParam`
 * in `backend/internal/api/handlers/auth/oauth_handlers.go`; the names on both
 * sides must stay the same.
 */
export const AUTH_ERROR_PARAM = 'error'
export const AUTH_ERROR_CODE_PARAM = 'error_code'

/**
 * Does this auth-page URL carry the OAuth callback's refusal of a deleted
 * account still inside its recovery window? The code is compared against one
 * fixed value and never rendered, so it needs no decoding.
 */
export function isAccountRecoverableRefusal(searchParams: {
  get: (name: string) => string | null
}): boolean {
  return (
    searchParams.get(AUTH_ERROR_CODE_PARAM) ===
    AuthErrorCode.ACCOUNT_RECOVERABLE
  )
}

/**
 * The auth-page href for an anonymous viewer a gated control turned away:
 * `buildAuthHref`'s destination plus the intent that opens Create account.
 *
 * `useAuthGatedAction` is the only module outside this one that names it,
 * which `authGateOwnership.test.ts` pins. A link a reader follows on purpose
 * to sign in names no intent, because nothing about it says they lack an
 * account.
 */
export function buildGatedAuthHref(
  returnTo: string,
  intent: AuthIntent
): string {
  return appendAuthParam(buildAuthHref(returnTo), AUTH_INTENT_PARAM, intent)
}

/**
 * Adds one parameter to an href `buildAuthHref` produced, which may or may not
 * already carry a query string. Every value passed here is a fixed token from
 * this module, so none needs encoding.
 */
function appendAuthParam(base: string, name: string, value: string): string {
  const separator = base.includes('?') ? '&' : '?'
  return `${base}${separator}${name}=${value}`
}

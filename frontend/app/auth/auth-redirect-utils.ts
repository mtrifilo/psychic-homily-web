/**
 * Reads `returnTo` back out of the auth page's URL and decides whether to
 * trust it. The other half of the contract, which builds those links, is
 * `buildAuthHref` in `lib/auth-href.ts`; change one and check the other.
 * `lib/auth-href.test.ts` pins the round trip between them.
 */

import {
  FALLBACK_RETURN_TO,
  isAuthPath,
  isOAuthLinkResultPair,
  withoutOAuthLinkResult,
} from '@/lib/auth-href'

/** The backend's `oauthReturnToMaxLength`. */
const RETURN_TO_MAX_LENGTH = 2048

/** Every character from "!" to "~": no space, control character or non-ASCII. */
const PRINTABLE_ASCII = /^[!-~]*$/

/** A `%` not followed by two hex digits. */
const MALFORMED_ESCAPE = /%(?![0-9A-Fa-f]{2})/

/**
 * Returns `rawReturnTo` when it is a path on this origin that a signed-in
 * reader may be sent to, less any OAuth link result in its query string, and
 * FALLBACK_RETURN_TO for anything else.
 *
 * The rules are `sanitizeOAuthReturnTo` in
 * `backend/internal/api/handlers/auth/oauth_return_to.go`, which is their
 * source of truth and documents why each exists. Google sign-in carries the
 * destination through that function, so the two refuse the same values and
 * every sign-in method lands in the same place. They are applied here in the
 * same order:
 *
 *   - at most RETURN_TO_MAX_LENGTH characters
 *   - one leading slash: "//host" is protocol-relative
 *   - printable ASCII only: no whitespace, control character or non-ASCII
 *   - no backslash in the path
 *   - no "." or ".." path segment, percent-encoded or not
 *   - no malformed percent escape in the path or the fragment
 *   - not the sign-in page or anything under it, judged on the decoded path
 *
 * A destination that passes them all loses its OAuth link result parameters
 * (`withoutOAuthLinkResult` in `lib/auth-href.ts`): Settings reads them once,
 * on the redirect that delivers them, and a returnTo still carrying one would
 * replay that result after a later sign-in. Every other parameter, and the
 * fragment, keep the spelling and order they arrived with.
 *
 * `backend/internal/api/handlers/auth/testdata/return_to_corpus.json` holds
 * the cases both sanitizers' tests assert, with one expected output each.
 */
export function sanitizeReturnTo(
  rawReturnTo: string | null | undefined
): string {
  const raw = rawReturnTo ?? ''
  // Go counts bytes and this counts UTF-16 code units. They differ only on
  // non-ASCII input, which the printable-ASCII rule refuses either way.
  if (raw === '' || raw.length > RETURN_TO_MAX_LENGTH) {
    return FALLBACK_RETURN_TO
  }
  if (!raw.startsWith('/') || raw.startsWith('//')) {
    return FALLBACK_RETURN_TO
  }
  if (!PRINTABLE_ASCII.test(raw)) {
    return FALLBACK_RETURN_TO
  }

  const path = raw.split(/[?#]/)[0]
  if (path.includes('\\')) {
    return FALLBACK_RETURN_TO
  }
  if (MALFORMED_ESCAPE.test(path) || path.split('/').some(isDotSegment)) {
    return FALLBACK_RETURN_TO
  }

  const fragmentStart = raw.indexOf('#')
  if (fragmentStart !== -1 && MALFORMED_ESCAPE.test(raw.slice(fragmentStart + 1))) {
    return FALLBACK_RETURN_TO
  }
  if (isAuthPath(decodeAsciiEscapes(path))) {
    return FALLBACK_RETURN_TO
  }
  return withoutOAuthLinkResultIn(raw)
}

/**
 * `returnTo` without the OAuth link result pairs of its query string, or
 * `returnTo` itself, byte for byte, when its query carries none.
 */
function withoutOAuthLinkResultIn(returnTo: string): string {
  const queryStart = returnTo.indexOf('?')
  const fragmentStart = returnTo.indexOf('#')
  if (queryStart === -1 || (fragmentStart !== -1 && fragmentStart < queryStart)) {
    return returnTo
  }
  const queryEnd = fragmentStart === -1 ? returnTo.length : fragmentStart
  if (!returnTo.slice(queryStart + 1, queryEnd).split('&').some(isOAuthLinkResultPair)) {
    return returnTo
  }
  // Handed over with its own "?", which withoutOAuthLinkResult removes, so a
  // first pair that begins with "?" keeps it.
  return (
    returnTo.slice(0, queryStart) +
    withoutOAuthLinkResult(returnTo.slice(queryStart, queryEnd)) +
    returnTo.slice(queryEnd)
  )
}

function isDotSegment(segment: string): boolean {
  const decoded = decodeAsciiEscapes(segment)
  return decoded === '.' || decoded === '..'
}

/**
 * Decodes the percent escapes of ASCII bytes and leaves the rest encoded.
 * Every value it is compared against (".", "..", the sign-in page's path) is
 * ASCII, so a byte left encoded changes no outcome, and `decodeURIComponent`
 * would throw on an escape that is not UTF-8 ("/a/%FF"), which Go decodes
 * and keeps.
 */
function decodeAsciiEscapes(path: string): string {
  return path.replace(/%([0-7][0-9A-Fa-f])/g, (_, hex: string) =>
    String.fromCharCode(Number.parseInt(hex, 16))
  )
}

export function safeDecodeQueryParam(
  rawValue: string | null | undefined
): string | null {
  if (!rawValue) {
    return null
  }

  try {
    return decodeURIComponent(rawValue)
  } catch {
    return rawValue
  }
}

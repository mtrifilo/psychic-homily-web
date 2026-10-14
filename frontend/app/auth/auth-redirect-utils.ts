/**
 * Reads `returnTo` back out of the auth page's URL and decides whether to
 * trust it. The other half of the contract, which builds those links, is
 * `buildAuthHref` in `lib/auth-href.ts` — change one and check the other.
 * `lib/auth-href.test.ts` pins the round trip between them.
 */

import { FALLBACK_RETURN_TO, isAuthPath } from '@/lib/auth-href'

const BASE_ORIGIN = 'https://psychichomily.com'

export function sanitizeReturnTo(
  rawReturnTo: string | null | undefined
): string {
  if (!rawReturnTo) {
    return FALLBACK_RETURN_TO
  }

  const trimmed = rawReturnTo.trim()
  if (!trimmed || !trimmed.startsWith('/') || trimmed.startsWith('//')) {
    return FALLBACK_RETURN_TO
  }

  try {
    const parsed = new URL(trimmed, BASE_ORIGIN)
    if (parsed.origin !== BASE_ORIGIN || isAuthPath(parsed.pathname)) {
      return FALLBACK_RETURN_TO
    }

    const destination = `${parsed.pathname}${parsed.search}${parsed.hash}`
    // The `//` test above runs on the RAW input; this one runs on the result,
    // and they are not the same test. URL normalization collapses `..`
    // segments, so `/..//evil.com` arrives past the raw check as a same-origin
    // URL whose pathname is `//evil.com`. Returned unchecked that is a
    // protocol-relative destination, and both sinks (`router.push` and the
    // interstitial's `<Link>`) navigate cross-origin on it.
    if (destination.startsWith('//')) {
      return FALLBACK_RETURN_TO
    }
    return destination
  } catch {
    return FALLBACK_RETURN_TO
  }
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

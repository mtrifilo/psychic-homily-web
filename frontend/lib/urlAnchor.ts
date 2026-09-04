/**
 * The three primitives every host-anchored render gate in this codebase is
 * built from: strip what the URL parser strips, parse an http(s) URL, and ask
 * whether the parsed host sits on an allowlisted base.
 *
 * They live together because the anchor rule is security-load-bearing and one
 * copy of it is the point. The REGISTRIES stay separate (a release link's
 * platforms are not a social column's hosts); only the parse and the anchor
 * converge here.
 *
 * Readers: lib/releaseLinks.ts and lib/socialLinks.ts. lib/bandcamp.ts keeps its
 * own stricter check, because it doubles as the SSRF guard for a server-side
 * fetch and so demands https alone.
 */

/**
 * Surrounding characters the WHATWG URL parser itself strips: C0 controls and
 * space, and nothing else.
 *
 * `String.trim()` is the wrong tool for deciding whether a stored value will
 * resolve, because it also strips U+00A0 and U+FEFF, which the URL parser
 * keeps. Leading, those make the whole value unparseable, so trimming with
 * `trim()` certifies an href that resolves same-origin. Trailing, they survive
 * into the path and the link does reach its destination, so refusing is
 * conservative rather than necessary; one rule for both is worth more than the
 * one row it costs.
 */
const URL_EDGE_WHITESPACE = /^[\u0000-\u0020]+|[\u0000-\u0020]+$/g

export function stripUrlWhitespace(raw: string): string {
  return raw.replace(URL_EDGE_WHITESPACE, '')
}

/** A parsed http or https URL, or null for anything else. */
export function parseHttpUrl(raw: string): URL | null {
  let parsed: URL
  try {
    parsed = new URL(raw)
  } catch {
    return null
  }
  if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') return null
  return parsed
}

/**
 * Whether a parsed URL's host is anchored to one of the bases.
 *
 * The leading dot is load-bearing: it rejects "notbandcamp.com" and
 * "bandcamp.com.evil.test" while accepting "<artist>.bandcamp.com".
 */
export function hostIsAnchored(parsed: URL, bases: readonly string[]): boolean {
  const host = parsed.hostname.toLowerCase()
  return bases.some(base => host === base || host.endsWith(`.${base}`))
}

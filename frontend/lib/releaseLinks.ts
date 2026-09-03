import { isBandcampReleaseUrl } from './bandcamp'

/**
 * The platforms a release's "Listen / Buy" link may name, the hosts each one's
 * URL may sit on, and whether the pickers offer it.
 *
 * This object is the single registry: the picker's options, the label printed
 * beside a link, and the gate that decides whether a stored row becomes an
 * `<a href>` all read it. Before PSY-1996 the picker and the label map were two
 * hand-written lists, and the display map already knew three platforms the
 * picker could not produce.
 *
 * `offered` is what keeps those two questions distinct without a second list.
 * "What may be stored and rendered" is wider than "what do we invite a curator
 * to type": the seed writes youtube_music rows nobody typed. The offered set is
 * the seven the picker carried before PSY-1996, unchanged; widening it is a
 * product call, not a consequence of merging the two lists. For amazon_music it
 * is also a correctness call, noted at its entry.
 *
 * The platform key is what a reader sees next to the URL, so an unanchored URL
 * is worse than a dead link: an arbitrary host wearing a name they trust.
 * Anchoring is on the parsed hostname, never a substring of the URL. A host
 * matches when it equals a base or is a subdomain of it, which is what covers
 * <artist>.bandcamp.com, geo.music.apple.com and www.discogs.com without
 * listing each.
 *
 * Key order is the order the pickers offer.
 *
 * CROSS-LANGUAGE MIRROR: releaseLinkPlatformHosts in
 * backend/internal/utils/release_link.go, where the reasoning behind each host
 * choice lives. Both sides assert this table against
 * backend/internal/utils/testdata/release_link_corpus.json, so a platform or
 * host added to one language and not the other fails the other language's suite
 * by name. `offered` is a UI concern and has no backend counterpart.
 */
export const RELEASE_LINK_PLATFORMS = {
  bandcamp: { label: 'Bandcamp', hosts: ['bandcamp.com'], offered: true },
  spotify: { label: 'Spotify', hosts: ['open.spotify.com'], offered: true },
  apple_music: {
    label: 'Apple Music',
    hosts: ['music.apple.com', 'itunes.apple.com'],
    offered: true,
  },
  youtube: { label: 'YouTube', hosts: ['youtube.com', 'youtu.be'], offered: true },
  // Offering this one would be wrong as well as new: the anchor is the US music
  // host, and Amazon Music's regional storefronts are separate registrable
  // domains, so the dropdown would invite URLs the gate refuses. Kept in the
  // registry because the label and the render gate still answer for rows that
  // exist.
  amazon_music: { label: 'Amazon Music', hosts: ['music.amazon.com'], offered: false },
  youtube_music: {
    label: 'YouTube Music',
    hosts: ['music.youtube.com'],
    offered: false,
  },
  deezer: { label: 'Deezer', hosts: ['deezer.com'], offered: false },
  discogs: { label: 'Discogs', hosts: ['discogs.com'], offered: true },
  tidal: { label: 'Tidal', hosts: ['tidal.com'], offered: true },
  soundcloud: { label: 'SoundCloud', hosts: ['soundcloud.com'], offered: true },
} as const satisfies Record<
  string,
  { label: string; hosts: readonly string[]; offered: boolean }
>

export type ReleaseLinkPlatform = keyof typeof RELEASE_LINK_PLATFORMS

/** Every platform key the gate knows, in registry order. */
export const RELEASE_LINK_PLATFORM_KEYS = Object.keys(
  RELEASE_LINK_PLATFORMS
) as ReleaseLinkPlatform[]

/** The keys the pickers offer, in the order they offer them. */
export const OFFERED_RELEASE_LINK_PLATFORM_KEYS = RELEASE_LINK_PLATFORM_KEYS.filter(
  key => RELEASE_LINK_PLATFORMS[key].offered
)

/**
 * Mirrors utils.MaxReleaseLinkURLLen. The column is TEXT, so this cap exists so
 * the write boundary and this gate agree about what fits rather than because
 * anything below objects.
 *
 * The two count differently: Go counts BYTES, this counts UTF-16 code units, so
 * a long multibyte URL can be refused by the writer and accepted here. That is
 * the safe direction (nothing storable becomes unrenderable) and the corpus pins
 * only the number.
 */
export const MAX_RELEASE_LINK_URL_LENGTH = 2048

/** The shape both the gate and the label lookup need from a stored row. */
export interface ReleaseLinkLike {
  platform: string
  url: string
}

/**
 * Only [a-z0-9.-]. Used by the write-side mirror alone, because it is a rule
 * about what may be STORED: a browser runs UTS-46 over a non-ASCII host and
 * several code points map to something else there (U+3002 becomes a label
 * separator, U+00AD is deleted) while Go keeps those bytes verbatim, so the two
 * would judge the same stored value differently. Restricting to the bytes a real
 * platform host is made of removes every such case.
 */
const ASCII_HOST = /^[a-z0-9.-]+$/

/** Printable ASCII, which is all a raw authority may be made of. */
const PRINTABLE_ASCII = /^[!-~]+$/

/**
 * Whitespace at either edge, under the union of what Go's strings.TrimSpace and
 * this language's trim() strip. Neither set contains the other: Go strips U+0085
 * and not U+FEFF, JavaScript the reverse. A value either would strip is one the
 * two layers would disagree about, so the write-side mirror refuses all of them.
 *
 * Both UI callers submit an already-trimmed URL, so in the app this rule is a
 * floor rather than something a curator will meet. It is here so the mirror is
 * complete for any caller that is not one of those two forms.
 */
const EDGE_WHITESPACE = /^[\s\u0085]|[\s\u0085]$/

/** C0 controls and DEL, which Go's url.Parse refuses anywhere in a URL. */
const C0_CONTROL = /[\u0000-\u001F\u007F]/

/**
 * The authority as WRITTEN: the bytes between "://" and the first "/", "?" or
 * "#", before any parser has touched them.
 *
 * This exists because `URL.hostname` is the WRONG input for a rule about what
 * Go will accept. By the time the WHATWG parser hands back a hostname it has
 * already run UTS-46 and folded the host to ASCII, so an ASCII test on it can
 * never see the non-ASCII bytes it is there to reject, and the hint approves
 * values the server refuses. Go sees the bytes; so does this.
 */
function rawAuthority(raw: string): string | null {
  const start = raw.indexOf('://')
  if (start < 0) return null
  const rest = raw.slice(start + 3)
  const end = rest.search(/[/?#]/)
  const authority = end < 0 ? rest : rest.slice(0, end)
  return authority || null
}

/**
 * Every registry key is ASCII, and the key is checked against this BEFORE it is
 * folded. Folding first would let the two languages disagree: Go's
 * strings.ToLower uses simple case mapping and turns "TİDAL" (U+0130) into
 * "tidal", while this language's SpecialCasing turns it into "ti̇dal" and
 * finds nothing, so the row stores with a 201 and renders no link.
 */
const ASCII_PLATFORM = /^[A-Za-z0-9_]+$/

/**
 * The registry entry for a platform, or null.
 *
 * Lowercases but does NOT trim, mirroring the Go lookup: casing has always named
 * the same platform (the enrichment writer's dedup index is on LOWER(platform)),
 * while a padded value is a different string that would be stored padded.
 */
function platformEntry(platform: string) {
  if (!ASCII_PLATFORM.test(platform)) return null
  const key = platform.toLowerCase()
  return Object.prototype.hasOwnProperty.call(RELEASE_LINK_PLATFORMS, key)
    ? RELEASE_LINK_PLATFORMS[key as ReleaseLinkPlatform]
    : null
}

/**
 * The label to print beside a link. Falls back to title-casing an unknown
 * platform so an admin surface can still name a legacy row it refuses to link.
 */
export function releaseLinkPlatformLabel(platform: string): string {
  return (
    platformEntry(platform)?.label ??
    platform
      .split('_')
      .map(w => w.charAt(0).toUpperCase() + w.slice(1))
      .join(' ')
  )
}

/**
 * Whether a parsed URL's host is anchored to one of the platform's bases.
 *
 * The leading dot is load-bearing: it rejects "notbandcamp.com" and
 * "bandcamp.com.evil.test" while accepting "<artist>.bandcamp.com".
 */
function hostIsAnchored(parsed: URL, bases: readonly string[]): boolean {
  const host = parsed.hostname.toLowerCase()
  return bases.some(base => host === base || host.endsWith(`.${base}`))
}

function parseHttpUrl(raw: string): URL | null {
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
 * The refusal to show beside a URL field someone is filling in, or null.
 *
 * This mirrors the WRITE gate, utils.ValidateReleaseLink, not the render gate
 * below: its job is to say in advance what the server will say, so it applies
 * the same rules. Notably it refuses a host a browser would normalize onto the
 * platform (a fullwidth letter, a soft hyphen), because the server does; the
 * render gate deliberately does not, and that asymmetry is the point of having
 * two functions rather than one.
 *
 * The same RULES, not the same sentence: the server's wording names the platform
 * key and echoes the submitted value, this one names the label a curator just
 * picked. Both name the hosts that work, which is the part a submitter acts on.
 *
 * An empty URL returns null: an untouched field is not yet a mistake. Whether
 * empty may be submitted is the caller's own concern.
 *
 * It deliberately says nothing about the PATH. A release link is "this release
 * on that platform" and each platform spells that differently, so a path rule
 * would refuse real links (locale-prefixed Spotify URLs, Apple Music's country
 * segment) to buy nothing: the host anchor already decides where a click lands.
 */
export function releaseLinkRefusal(link: ReleaseLinkLike): string | null {
  if (!link.platform.trim()) return 'Platform is required'
  const entry = platformEntry(link.platform)
  if (!entry) {
    return `Platform must be one of: ${RELEASE_LINK_PLATFORM_KEYS.join(', ')}`
  }
  if (!link.url.trim()) return null
  if (link.url.length > MAX_RELEASE_LINK_URL_LENGTH) {
    return `URL must be ${MAX_RELEASE_LINK_URL_LENGTH} characters or fewer`
  }

  const onPlatform = (() => {
    // Judged exactly as it would be stored, like the server. Go's TrimSpace and
    // this language's trim() do not cover the same code points, so the edges are
    // tested against the union of both: whatever either would strip is a value
    // the two layers would disagree about.
    if (EDGE_WHITESPACE.test(link.url)) return false
    // Go's url.Parse refuses any C0 control anywhere; this parser strips the
    // surrounding ones and would otherwise let them through.
    if (C0_CONTROL.test(link.url)) return false

    // The authority AS WRITTEN, which is what Go judges. Non-ASCII here is what
    // UTS-46 folds away, a backslash and a percent escape are what Go's parser
    // refuses outright, and an "@" is userinfo, which no real platform URL
    // carries and which the release card would print under the platform's name.
    const authority = rawAuthority(link.url)
    if (authority === null) return false
    if (!PRINTABLE_ASCII.test(authority)) return false
    if (/[\\@%]/.test(authority)) return false

    const parsed = parseHttpUrl(link.url)
    if (parsed === null) return false
    const host = parsed.hostname.toLowerCase()
    if (!ASCII_HOST.test(host)) return false
    if (host.split('.').some(label => label.startsWith('xn--'))) return false
    if (parsed.port !== '' && Number(parsed.port) > MAX_TCP_PORT) return false
    return hostIsAnchored(parsed, entry.hosts)
  })()

  return onPlatform
    ? null
    : `${entry.label} link must be an http or https URL on ${entry.hosts.join(' or ')}`
}

/**
 * Where the WHATWG URL parser stops accepting a port. It throws above this, so
 * in practice `new URL` has already refused; the explicit bound keeps the
 * mirror readable next to the Go one.
 */
const MAX_TCP_PORT = 65535

/**
 * Surrounding characters the WHATWG URL parser itself strips: C0 controls and
 * space, and nothing else.
 *
 * `String.trim()` is the wrong tool for deciding whether a stored value will
 * resolve, because it also strips U+00A0 and U+FEFF, which the URL parser keeps.
 * LEADING, those make the whole value unparseable, so trimming with `trim()`
 * certified an href that resolves same-origin and 404s. Trailing, they survive
 * into the path and the link does reach the platform, so refusing the row is
 * conservative rather than necessary; one rule for both is worth more than the
 * one row it costs.
 */
function stripUrlWhitespace(raw: string): string {
  return raw.replace(/^[\u0000-\u0020]+|[\u0000-\u0020]+$/g, '')
}

/**
 * The render gate: whether a stored row may become an `<a href>`.
 *
 * The backend refuses to store anything this refuses, so a row written from now
 * on always produces a link. Legacy rows are why the read side asks at all:
 * nothing backfills, so a row written before the gate existed keeps its value
 * and renders no link.
 *
 * Where the two sides differ, this one is the LENIENT side, never the stricter.
 * It asks only what a browser will do with the value: parse it, and see whether
 * the host it resolves to is on the platform. So a legacy row whose host a
 * browser normalizes onto the platform (a UTS-46 mapping, a punycode label)
 * keeps its link, where the writer refuses to store any more of them. The shared
 * corpus records each such case and why the value still lands on-platform.
 *
 * Being lenient is not the same as being loose. It will not certify a value the
 * browser would refuse to follow, which is why the whitespace it strips is
 * exactly the whitespace the URL parser strips.
 *
 * And it refuses userinfo, the one shape where "will a browser reach the
 * platform" is the wrong question. The card prints the stored URL as its caption
 * and truncates from the right, so
 * "https://your-account-is-suspended.example.com@open.spotify.com/album/x" reads
 * as another domain with the real host cut off the end. What separates that from
 * any long hostname is that userinfo is not part of the host in ANY parser: it
 * is text a browser discards, so the caption names a domain the click does not
 * go near. A misleading SUBDOMAIN is a different thing, because that is where
 * the click genuinely goes, and nothing here closes it: the anchor buys "on the
 * platform", never "vouched for by it".
 */
export function isRenderableReleaseLink(link: ReleaseLinkLike): boolean {
  const entry = platformEntry(link.platform)
  if (!entry) return false
  const url = stripUrlWhitespace(link.url)
  if (!url || url.length > MAX_RELEASE_LINK_URL_LENGTH) return false
  const parsed = parseHttpUrl(url)
  if (parsed === null) return false
  if (parsed.username !== '' || parsed.password !== '') return false
  return hostIsAnchored(parsed, entry.hosts)
}

/**
 * The stored rows that may be rendered as links, in their stored order.
 *
 * Every surface that turns a release link into an href takes its list from here
 * rather than testing rows itself, so a new surface inherits the gate instead of
 * silently skipping it.
 */
export function renderableReleaseLinks<T extends ReleaseLinkLike>(
  links: readonly T[] | null | undefined
): T[] {
  return (links ?? []).filter(isRenderableReleaseLink)
}

/**
 * The Bandcamp release URL to feed the embed (PSY-1187), or null.
 *
 * Only `/album/<slug>` and `/track/<slug>` pages resolve to a playable iframe; a
 * bare profile root does not. Picking here rather than inside MusicEmbed means
 * the resolver fetch only fires when a player can render, and a profile-only
 * link is still shown by the "Listen / Buy" card.
 *
 * The path test is `isBandcampReleaseUrl`, which reads `pathname`. A substring
 * test over the whole URL would select a link whose QUERY happens to contain
 * "/album/".
 */
export function findBandcampEmbedUrl(
  links: readonly ReleaseLinkLike[] | null | undefined
): string | null {
  const link = renderableReleaseLinks(links).find(
    l => l.platform.toLowerCase() === 'bandcamp' && isBandcampReleaseUrl(l.url)
  )
  return link?.url ?? null
}

/**
 * The Spotify URL to feed the embed (PSY-1195), or null.
 *
 * Any renderable spotify-platform link qualifies: MusicEmbed runs
 * `parseSpotifyEmbed` on it, which accepts only album/track/artist URLs, so a
 * search or playlist link falls back to the plain "Listen / Buy" card. When a
 * release also has a Bandcamp release link, MusicEmbed's own priority renders
 * the Bandcamp embed, so passing both is safe.
 */
export function findSpotifyEmbedUrl(
  links: readonly ReleaseLinkLike[] | null | undefined
): string | null {
  const link = renderableReleaseLinks(links).find(
    l => l.platform.toLowerCase() === 'spotify'
  )
  return link?.url ?? null
}

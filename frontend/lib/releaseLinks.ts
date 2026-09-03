import { isBandcampReleaseUrl } from './bandcamp'

/**
 * The platforms a release's "Listen / Buy" link may name, and the hosts each
 * one's URL may sit on.
 *
 * This object is the single registry: the picker's options, the label printed
 * beside a link, and the gate that decides whether a stored row becomes an
 * `<a href>` all read it. Before PSY-1996 the picker and the label map were two
 * hand-written lists, and the display map already knew three platforms the
 * picker could not produce.
 *
 * The platform key is what a reader sees next to the URL, so an unanchored URL
 * is worse than a dead link: an arbitrary host wearing a name they trust.
 * Anchoring is on the parsed hostname, never a substring of the URL. A host
 * matches when it equals a base or is a subdomain of it, which is what covers
 * open.spotify.com, <artist>.bandcamp.com, itunes.apple.com and www.discogs.com
 * without listing each. youtu.be is YouTube's own share host; no other
 * redirector belongs here, because one that cannot be shown statically to land
 * on-platform defeats the anchor.
 *
 * Key order is the order the picker offers.
 *
 * CROSS-LANGUAGE MIRROR: releaseLinkPlatformHosts in
 * backend/internal/utils/release_link.go, where the reasoning behind each rule
 * lives. Both sides assert this table against
 * backend/internal/utils/testdata/release_link_corpus.json, so a platform or
 * host added to one language and not the other fails the other language's suite
 * by name.
 */
export const RELEASE_LINK_PLATFORMS = {
  bandcamp: { label: 'Bandcamp', hosts: ['bandcamp.com'] },
  spotify: { label: 'Spotify', hosts: ['spotify.com'] },
  apple_music: { label: 'Apple Music', hosts: ['apple.com'] },
  youtube: { label: 'YouTube', hosts: ['youtube.com', 'youtu.be'] },
  youtube_music: { label: 'YouTube Music', hosts: ['youtube.com'] },
  soundcloud: { label: 'SoundCloud', hosts: ['soundcloud.com'] },
  tidal: { label: 'Tidal', hosts: ['tidal.com'] },
  deezer: { label: 'Deezer', hosts: ['deezer.com'] },
  amazon_music: { label: 'Amazon Music', hosts: ['amazon.com'] },
  discogs: { label: 'Discogs', hosts: ['discogs.com'] },
} as const satisfies Record<string, { label: string; hosts: readonly string[] }>

export type ReleaseLinkPlatform = keyof typeof RELEASE_LINK_PLATFORMS

/** Every platform key, in picker order. */
export const RELEASE_LINK_PLATFORM_KEYS = Object.keys(
  RELEASE_LINK_PLATFORMS
) as ReleaseLinkPlatform[]

/**
 * Mirrors utils.MaxReleaseLinkURLLen. The column is TEXT, so this cap exists so
 * the write boundary and this gate agree about what fits rather than because
 * anything below objects.
 */
export const MAX_RELEASE_LINK_URL_LENGTH = 2048

/** The shape both the gate and the label lookup need from a stored row. */
export interface ReleaseLinkLike {
  platform: string
  url: string
}

/**
 * Only [a-z0-9.-], so the suffix anchor below means the same thing here and in
 * Go. A browser runs UTS-46 over a non-ASCII host and several code points map
 * to something else there (U+3002 becomes a label separator, U+00AD is deleted);
 * Go keeps those bytes verbatim. Restricting the host to the bytes a real
 * platform host is made of removes every such case on both sides.
 */
const ASCII_HOST = /^[a-z0-9.-]+$/

/**
 * The registry entry for a platform, or null.
 *
 * Lowercases but does NOT trim, mirroring the Go lookup: casing has always named
 * the same platform (the enrichment writer's dedup index is on LOWER(platform)),
 * while a padded value is a different string that would be stored padded.
 */
function platformEntry(platform: string) {
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
 * The single statement of what a release link may be, as the sentence to show
 * whoever wrote it, or null when the pair is fine.
 *
 * The rules mirror utils.ValidateReleaseLink in Go, judged by this language's
 * URL parser; the shared corpus records every input where the two parsers
 * disagree and why each is safe. The messages name the accepted value rather
 * than the rule that failed, because that is the only form a submitter can act
 * on.
 *
 * It deliberately says nothing about the PATH. A release link is "this release
 * on that platform" and each platform spells that differently, so a path rule
 * would refuse real links (locale-prefixed Spotify URLs, Apple Music's country
 * segment) to buy nothing: the host anchor already decides where a click lands.
 * The path-strict rules are the embed parsers (isBandcampReleaseUrl,
 * parseSpotifyEmbed), which answer a narrower question about one URL.
 */
function releaseLinkFailure(link: ReleaseLinkLike): string | null {
  const entry = platformEntry(link.platform)
  if (!entry) {
    return `Platform must be one of: ${RELEASE_LINK_PLATFORM_KEYS.join(', ')}`
  }
  if (!link.url.trim()) return 'URL is required'
  if (link.url.length > MAX_RELEASE_LINK_URL_LENGTH) {
    return `URL must be ${MAX_RELEASE_LINK_URL_LENGTH} characters or fewer`
  }

  const hostAnchored = (() => {
    // Judged exactly as stored: this parser trims before resolving, so a value
    // that only validates after trimming is one the two layers disagree about.
    if (link.url !== link.url.trim()) return false
    let parsed: URL
    try {
      parsed = new URL(link.url)
    } catch {
      return false
    }
    if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') return false
    const host = parsed.hostname.toLowerCase()
    if (!ASCII_HOST.test(host)) return false
    // The leading dot is load-bearing: it rejects "notbandcamp.com" and
    // "bandcamp.com.evil.test" while accepting "<artist>.bandcamp.com".
    return entry.hosts.some(base => host === base || host.endsWith(`.${base}`))
  })()

  return hostAnchored
    ? null
    : `${entry.label} link must be an http or https URL on ${entry.hosts.join(' or ')}`
}

/**
 * The refusal to show beside a URL field the user is still filling in, or null.
 *
 * The one difference from the gate: an empty field is not yet a mistake, so it
 * says nothing. Whether empty may be submitted is the caller's own concern.
 */
export function releaseLinkRefusal(link: ReleaseLinkLike): string | null {
  return link.url.trim() ? releaseLinkFailure(link) : null
}

/**
 * The predicate for what a release external link is allowed to be. The backend
 * gates every write path on the same rules, so a stored row always produces a
 * link and a row that would not produce a link is never stored.
 *
 * Legacy rows are the reason the read side asks at all: nothing backfills, so a
 * row written before the gate existed keeps its value and renders no link.
 */
export function isRenderableReleaseLink(link: ReleaseLinkLike): boolean {
  return releaseLinkFailure(link) === null
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

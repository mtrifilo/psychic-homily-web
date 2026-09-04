/**
 * The eight social columns an artist, venue, label or festival carries, the
 * hosts each one's value may resolve to, and where a bare handle goes.
 *
 * The key is printed beside the value as a platform's name (a glyph and an
 * accessible label on the entity pages, a `sameAs` identity claim in the
 * structured data), so a value on an arbitrary host is worse than a dead link:
 * it is a stranger's page wearing a name the reader trusts. `hosts` is what
 * decides where a click lands, anchored on the PARSED hostname, never on a
 * substring of the value.
 *
 * A host matches when it equals a base or is a subdomain of it, which is what
 * covers open.spotify.com, <artist>.bandcamp.com, m.facebook.com and
 * music.youtube.com without listing each.
 *
 * `hosts: null` means the field makes no platform claim and accepts any host.
 * `website` is the only one, and it is the escape hatch that lets the anchored
 * seven stay narrow.
 *
 * `handleBase` is where a legacy bare handle resolves. It is separate from
 * `hosts` because the two answer different questions: `hosts` is the whole
 * domain a value may sit anywhere under, `handleBase` is the single URL a
 * handle is appended to. Bandcamp has none because its account URL is a
 * subdomain rather than a path, so there is nothing to append a handle to.
 *
 * Key order is render order.
 *
 * CROSS-LANGUAGE MIRROR: socialHostSuffixes in backend/internal/utils/url.go,
 * where the reasoning behind each host choice lives. Both sides assert this
 * table against backend/internal/utils/testdata/social_link_corpus.json, so a
 * field or a host added to one language and not the other fails the other
 * language's suite by name. `label` and `handleBase` are render concerns and
 * have no backend counterpart.
 */
export const SOCIAL_LINK_PLATFORMS = {
  website: { label: 'Website', hosts: null, handleBase: null },
  instagram: {
    label: 'Instagram',
    hosts: ['instagram.com'],
    handleBase: 'https://instagram.com/',
  },
  facebook: {
    label: 'Facebook',
    hosts: ['facebook.com', 'fb.com'],
    handleBase: 'https://facebook.com/',
  },
  twitter: {
    label: 'Twitter/X',
    hosts: ['twitter.com', 'x.com'],
    handleBase: 'https://twitter.com/',
  },
  youtube: {
    label: 'YouTube',
    hosts: ['youtube.com', 'youtu.be'],
    handleBase: 'https://youtube.com/',
  },
  spotify: {
    label: 'Spotify',
    // The anchor is the whole domain, matching the backend, while a handle
    // resolves under the web player: open.spotify.com is a subdomain of the
    // anchor, so both agree about where a click lands.
    hosts: ['spotify.com'],
    handleBase: 'https://open.spotify.com/',
  },
  bandcamp: { label: 'Bandcamp', hosts: ['bandcamp.com'], handleBase: null },
  soundcloud: {
    label: 'SoundCloud',
    hosts: ['soundcloud.com'],
    handleBase: 'https://soundcloud.com/',
  },
} as const satisfies Record<
  string,
  { label: string; hosts: readonly string[] | null; handleBase: string | null }
>

export type SocialLinkPlatform = keyof typeof SOCIAL_LINK_PLATFORMS

/** Every social field the gate knows, in render order. */
export const SOCIAL_LINK_PLATFORM_KEYS = Object.keys(
  SOCIAL_LINK_PLATFORMS
) as SocialLinkPlatform[]

/** The fields anchored to a platform's hosts, in render order. */
export const ANCHORED_SOCIAL_LINK_PLATFORM_KEYS =
  SOCIAL_LINK_PLATFORM_KEYS.filter(key => SOCIAL_LINK_PLATFORMS[key].hosts !== null)

/** The stored shape every consumer of these columns passes around. */
export type SocialLinkValues = Partial<
  Record<SocialLinkPlatform, string | null | undefined>
>

/**
 * Surrounding characters the WHATWG URL parser itself strips: C0 controls and
 * space, and nothing else.
 *
 * `String.trim()` is the wrong tool for deciding whether a stored value will
 * resolve, because it also strips U+00A0 and U+FEFF, which the URL parser
 * keeps. Leading, those make the whole value unparseable, so trimming with
 * `trim()` would certify an href that resolves same-origin.
 */
function stripUrlWhitespace(raw: string): string {
  return raw.replace(/^[\u0000-\u0020]+|[\u0000-\u0020]+$/g, '')
}

/**
 * A value that already carries its own scheme.
 *
 * Case-insensitive because Go lowercases the scheme it parses, so "HTTPS://x"
 * clears the write boundary and is a value this side has to read back.
 */
const HAS_HTTP_SCHEME = /^https?:\/\//i

/**
 * A stored value as a URL string, applying the tolerance the legacy rows need.
 *
 * Three shapes reach this: a full URL, a scheme-less domain, and a bare handle.
 * Only the first is storable today; the other two are why the tolerance exists,
 * and neither is trusted by it. Every branch's output goes through the host
 * anchor below, so widening the tolerance can only ever produce a URL, never a
 * link off the platform.
 */
function normalizeSocialValue(raw: string, handleBase: string | null): string {
  if (HAS_HTTP_SCHEME.test(raw)) {
    return raw
  }
  // A domain-shaped value with no scheme. `https://` rather than `//` so the
  // result is absolute in the parser as well as in a browser.
  if (
    raw.includes('.') &&
    (raw.includes('/') || raw.includes('.com') || raw.includes('.org'))
  ) {
    return `https://${raw}`
  }
  // A handle never contains a colon, so a value carrying one is a URL or a URI
  // and is judged as one rather than pasted onto the platform's base. Without
  // this, "javascript:alert(1)" resolves to a real on-platform 404 and renders
  // a link, and a "spotify:artist:x" URI renders one too.
  if (handleBase && !raw.includes(':')) {
    return `${handleBase}${raw.startsWith('@') ? raw.slice(1) : raw}`
  }
  return `https://${raw}`
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
 * Whether a parsed URL's host is anchored to one of the bases.
 *
 * The leading dot is load-bearing: it rejects "notinstagram.com" and
 * "instagram.com.evil.test" while accepting "<artist>.bandcamp.com".
 */
function hostIsAnchored(parsed: URL, bases: readonly string[]): boolean {
  const host = parsed.hostname.toLowerCase()
  return bases.some(base => host === base || host.endsWith(`.${base}`))
}

/**
 * The href a stored social value may become, or null.
 *
 * This is the read half of the host anchor the write boundary applies
 * (utils.ValidateSocialHost). The backend refuses to store an off-platform host
 * in any of the anchored seven, so a row written from now on always produces a
 * link; a row written before that gate existed keeps its value, and this is
 * what stops it becoming an outbound link under a platform's name. Nothing
 * backfills, so the stored value is unchanged either way.
 *
 * The href returned is the exact string that was parsed, so the anchor is
 * checked against the same parse a browser performs on the attribute. Returning
 * anything else would let the two disagree.
 *
 * Where the two languages differ, this side is the LENIENT one. It asks only
 * what a browser will do with the value: parse it, and see where the host it
 * resolves to sits. A legacy handle or scheme-less domain that lands on the
 * platform therefore keeps its link, while the writer stores no more of them.
 *
 * Userinfo is deliberately NOT refused. The anchor reads `hostname`, which
 * excludes it in every parser, so a value carrying userinfo still resolves to
 * the anchored host; and no surface here prints the URL as text, so there is no
 * caption for a userinfo prefix to misread. Refusing it would make a value the
 * write boundary accepts render nothing.
 *
 * What the anchor buys is "on the platform", never "vouched for by it": a
 * misleading subdomain is where the click genuinely goes, and nothing here
 * closes that.
 */
export function socialLinkHref(
  platform: string,
  value: string | null | undefined
): string | null {
  if (!Object.prototype.hasOwnProperty.call(SOCIAL_LINK_PLATFORMS, platform)) {
    return null
  }
  const entry = SOCIAL_LINK_PLATFORMS[platform as SocialLinkPlatform]
  if (typeof value !== 'string') return null
  const raw = stripUrlWhitespace(value)
  if (!raw) return null

  const candidate = normalizeSocialValue(raw, entry.handleBase)
  const parsed = parseHttpUrl(candidate)
  if (parsed === null) return null
  if (entry.hosts === null) return candidate
  return hostIsAnchored(parsed, entry.hosts) ? candidate : null
}

/** One stored column that survived the gate. */
export interface RenderableSocialLink {
  platform: SocialLinkPlatform
  label: string
  href: string
}

/**
 * The stored columns that may become links, in render order.
 *
 * Every surface that turns a social column into an href or a `sameAs` entry
 * takes its list from here rather than testing the columns itself, so a new
 * surface inherits the gate instead of silently skipping it.
 */
export function renderableSocialLinks(
  social: SocialLinkValues | null | undefined
): RenderableSocialLink[] {
  if (!social) return []
  const links: RenderableSocialLink[] = []
  for (const platform of SOCIAL_LINK_PLATFORM_KEYS) {
    const href = socialLinkHref(platform, social[platform])
    if (href) {
      links.push({ platform, label: SOCIAL_LINK_PLATFORMS[platform].label, href })
    }
  }
  return links
}

/**
 * Whether any stored column will render.
 *
 * The heading gate for the surfaces that print one above the links. It asks the
 * same question the render does, so a row whose every value is refused shows no
 * heading rather than a heading over nothing.
 */
export function hasRenderableSocialLink(
  social: SocialLinkValues | null | undefined
): boolean {
  return renderableSocialLinks(social).length > 0
}

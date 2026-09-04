import { hostIsAnchored, parseHttpUrl, stripUrlWhitespace } from './urlAnchor'

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
 * The `bandcamp` and `spotify` columns have a SECOND reader: MusicEmbed turns
 * them into a player or a "Listen on" link through lib/bandcamp.ts and
 * lib/spotify.ts, which are stricter (https only, and a path rule) because they
 * also guard a server-side fetch. Widening a host here does not widen those.
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
    // anchor, so both agree about where a click lands. RELEASE_LINK_PLATFORMS
    // anchors spotify to open.spotify.com alone, because a release link names
    // one album on the player; a profile column is wider on purpose.
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

/** The stored shape every consumer of these columns passes around. */
export type SocialLinkValues = Partial<
  Record<SocialLinkPlatform, string | null | undefined>
>

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
function normalizeSocialValue(
  raw: string,
  handleBase: string | null
): string | null {
  if (HAS_HTTP_SCHEME.test(raw)) return raw
  if (handleBase && isHandleShaped(raw)) {
    return `${handleBase}${raw.startsWith('@') ? raw.slice(1) : raw}`
  }
  // A scheme-less value is repaired only when it is domain-shaped. `https://`
  // rather than `//` so the result is absolute to the parser as well as to a
  // browser, and so no branch here can produce a non-http scheme.
  //
  // The dot is what separates a domain from a value that is neither URL nor
  // handle: without it a stored "123" repairs to `https://123`, which every
  // browser resolves to the host 0.0.0.123.
  //
  // The dot is a floor, not a proof of a domain. A dotted numeric value is
  // still read as an IPv4 address: "0x7f.1" resolves to 127.0.0.1 and "1.5" to
  // 1.0.0.5. On an ANCHORED field the host anchor below refuses every one of
  // them; on an unanchored field the result is a link to an address on the
  // reader's own network, which no gate here can distinguish from a deliberate
  // one.
  return raw.includes('.') ? `https://${raw}` : null
}

/**
 * Whether a scheme-less value is a handle rather than a URL someone left the
 * scheme off.
 *
 * A domain-shaped value is not a handle: "instagram.com/calexico" belongs under
 * its own host, not appended to one. Nor is anything carrying a colon, which is
 * either a scheme or a port; without that rule "javascript:alert(1)" in the
 * instagram column resolves to a real on-platform 404 and renders a link, and a
 * "spotify:artist:x" URI renders one too.
 */
function isHandleShaped(raw: string): boolean {
  if (raw.includes(':')) return false
  // Lowercased first: a host is case-insensitive, so "EVIL.COM" and "evil.com"
  // must take the same branch rather than one being repaired into a URL and
  // the other pasted onto the platform's base.
  const value = raw.toLowerCase()
  return !(
    value.includes('.') &&
    (value.includes('/') || value.includes('.com') || value.includes('.org'))
  )
}

/**
 * The href a stored social value may become, or null.
 *
 * This is the read half of the host anchor the write boundary applies
 * (utils.ValidateSocialHost). The backend refuses to store an off-platform host
 * in any of the anchored seven; a row written before that gate existed keeps
 * its value, and this is what stops it becoming an outbound link under a
 * platform's name. Nothing backfills, so the stored value is unchanged either
 * way.
 *
 * The two sides are not identical, and the shared corpus is where the gap is
 * written down: its `storableButUnrenderable` bucket is every shape the write
 * boundary accepts and this refuses.
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
 * Userinfo is refused, as it is by the release-link gate. A browser discards
 * it, so the click does land on the anchored host, but it is attacker-chosen
 * text that is not part of the host in any parser: it reads as a domain in a
 * status bar truncated from the right, and `sameAs` publishes it verbatim as
 * part of an identity claim about the entity. The write boundary accepts it, so
 * this is one of the shapes the corpus records as storable and not rendered.
 *
 * What the anchor buys is "on the platform", never "vouched for by it". A
 * misleading subdomain is where the click genuinely goes and nothing here
 * closes that, and neither does an on-platform redirector: `l.facebook.com` and
 * `youtube.com/redirect` are anchored hosts that forward off-platform. Closing
 * those needs a path rule and a shim denylist this deliberately has not got.
 */
export function socialLinkHref(
  platform: SocialLinkPlatform,
  value: string | null | undefined
): string | null {
  // The own-property test is not redundant with the type: a festival's `social`
  // is free-form JSONB, so a key that is not a platform reaches here at
  // runtime, and an inherited key ("constructor") would otherwise resolve to a
  // function.
  if (!Object.prototype.hasOwnProperty.call(SOCIAL_LINK_PLATFORMS, platform)) {
    return null
  }
  const entry = SOCIAL_LINK_PLATFORMS[platform]
  if (typeof value !== 'string') return null
  const raw = stripUrlWhitespace(value)
  if (!raw) return null

  const candidate = normalizeSocialValue(raw, entry.handleBase)
  if (candidate === null) return null
  const parsed = parseHttpUrl(candidate)
  if (parsed === null) return null
  if (parsed.username !== '' || parsed.password !== '') return null
  if (entry.hosts === null) return candidate
  return hostIsAnchored(parsed, entry.hosts) ? candidate : null
}

/**
 * Whether a free-form key names one of the columns this registry answers for.
 *
 * `radio_stations.social` is JSONB with arbitrary operator-chosen keys, and the
 * key is printed as the link's visible label, so a key the registry does not
 * know is a claim nothing here can check. The caller renders nothing for one
 * rather than guessing.
 */
export function isSocialLinkPlatform(key: string): key is SocialLinkPlatform {
  return Object.prototype.hasOwnProperty.call(SOCIAL_LINK_PLATFORMS, key)
}

/**
 * The href a stored value may become on a column that makes no platform claim.
 *
 * A station's `website` and `donation_url` are operator-entered free text on any
 * host, so there is no platform to anchor to and what is left is the tolerance
 * plus the parse: a scheme-less domain is repaired, and what survives is an
 * absolute http(s) URL with no userinfo, or nothing. It is the `website`
 * field's rule under a name that says what the caller is asking, so the two
 * cannot drift.
 *
 * A value that fails renders no link at all. That is better than the two
 * alternatives on these surfaces: a relative or unusable href, or a permanently
 * greyed bracket that reads as a disabled feature rather than as bad data.
 */
export function unanchoredLinkHref(
  value: string | null | undefined
): string | null {
  return socialLinkHref('website', value)
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
 * Every surface that turns one of THESE columns into an href or a `sameAs`
 * entry takes its list from here rather than testing the columns itself, so a
 * new surface inherits the gate instead of silently skipping it.
 *
 * A row is a fixed set of typed columns. `radio_stations.social` is free-form
 * JSONB whose keys are chosen by an operator, so it is not a row this can walk:
 * that caller asks `isSocialLinkPlatform` per key and `socialLinkHref` per
 * value instead.
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
  if (!social) return false
  return SOCIAL_LINK_PLATFORM_KEYS.some(
    platform => socialLinkHref(platform, social[platform]) !== null
  )
}

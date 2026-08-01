/**
 * JSON-LD Structured Data Helpers for SEO
 *
 * These helpers generate schema.org structured data that can be
 * embedded in pages to improve search engine understanding.
 */

import { resolveShowTimezone } from '@/lib/utils/formatters'
import { toZonedISOString } from '@/lib/utils/timeUtils'
import { SITE_DESCRIPTION, SITE_URL } from '@/lib/seo/siteMetadata'

export interface OrganizationSchema {
  '@context': 'https://schema.org'
  '@type': 'Organization'
  name: string
  url: string
  description?: string
  logo?: string
  sameAs?: string[]
}

export interface WebSiteSchema {
  '@context': 'https://schema.org'
  '@type': 'WebSite'
  name: string
  url: string
}

export interface BreadcrumbListSchema {
  '@context': 'https://schema.org'
  '@type': 'BreadcrumbList'
  itemListElement: Array<{
    '@type': 'ListItem'
    position: number
    name: string
    item: string
  }>
}

export interface MusicEventSchema {
  '@context': 'https://schema.org'
  '@type': 'MusicEvent'
  name: string
  startDate: string
  description?: string
  eventStatus: string
  eventAttendanceMode: string
  location: {
    '@type': 'MusicVenue'
    name: string
    url?: string
    address?: {
      '@type': 'PostalAddress'
      streetAddress?: string
      addressLocality?: string
      addressRegion?: string
      postalCode?: string
      addressCountry?: string
    }
  }
  performer?: Array<{
    '@type': 'MusicGroup'
    name: string
    url?: string
    sameAs?: string[]
  }>
  organizer?: {
    '@type': 'Organization'
    name: string
    url: string
  }
  offers?: {
    '@type': 'Offer'
    price?: number
    priceCurrency?: string
    availability?: string
    url?: string
  }
  image?: string[]
  url?: string
}

export interface BlogPostingSchema {
  '@context': 'https://schema.org'
  '@type': 'BlogPosting'
  headline: string
  datePublished: string
  dateModified?: string
  description?: string
  author?: {
    '@type': 'Person' | 'Organization'
    name: string
  }
  url?: string
}

export interface MusicVenueSchema {
  '@context': 'https://schema.org'
  '@type': 'MusicVenue'
  name: string
  address?: {
    '@type': 'PostalAddress'
    streetAddress?: string
    addressLocality?: string
    addressRegion?: string
    postalCode?: string
  }
  url?: string
}

export interface MusicGroupSchema {
  '@context': 'https://schema.org'
  '@type': 'MusicGroup'
  name: string
  url?: string
  sameAs?: string[]
  foundingLocation?: {
    '@type': 'Place'
    address?: {
      '@type': 'PostalAddress'
      addressLocality?: string
      addressRegion?: string
    }
  }
}

export interface MusicRecordingSchema {
  '@context': 'https://schema.org'
  '@type': 'MusicRecording'
  name: string
  byArtist?: {
    '@type': 'MusicGroup'
    name: string
  }
  datePublished?: string
  url?: string
}

export interface ItemListSchema {
  '@context': 'https://schema.org'
  '@type': 'ItemList'
  name?: string
  description?: string
  numberOfItems: number
  itemListElement: Array<{
    '@type': 'ListItem'
    position: number
    url: string
    name?: string
  }>
}

/**
 * Generate Organization schema for the site
 */
export function generateOrganizationSchema(): OrganizationSchema {
  return {
    '@context': 'https://schema.org',
    '@type': 'Organization',
    name: 'Psychic Homily',
    url: SITE_URL,
    description: SITE_DESCRIPTION,
    logo: `${SITE_URL}/og-image.jpg`,
  }
}

/**
 * Generate WebSite schema for the homepage
 */
export function generateWebSiteSchema(): WebSiteSchema {
  return {
    '@context': 'https://schema.org',
    '@type': 'WebSite',
    name: 'Psychic Homily',
    url: SITE_URL,
  }
}

/**
 * Generate BreadcrumbList schema for detail pages
 */
export function generateBreadcrumbSchema(
  items: Array<{ name: string; url: string }>
): BreadcrumbListSchema {
  return {
    '@context': 'https://schema.org',
    '@type': 'BreadcrumbList',
    itemListElement: items.map((item, index) => ({
      '@type': 'ListItem' as const,
      position: index + 1,
      name: item.name,
      item: item.url,
    })),
  }
}

/**
 * Pick the URL an `Offer` should point at: the show's ticket link when it has
 * one, the show's own page otherwise.
 *
 * `ticket_url` is free text — the submit form and the ingest CLI both accept a
 * bare host — so it is normalized before being trusted. A value that does not
 * resolve to an absolute http(s) URL falls back to the canonical page instead
 * of being emitted: a malformed offer URL is worse for a crawler than one that
 * merely points at the wrong kind of page, and the same field also carries
 * `mailto:`/`tel:` values that no crawler can buy a ticket through.
 *
 * The parsed URL is used only to validate; the candidate string is what gets
 * emitted, so a caller's ticket link is never silently rewritten.
 */
function resolveOfferUrl(
  ticketUrl: string | undefined,
  slug: string | undefined
): string | undefined {
  const canonical = slug ? `${SITE_URL}/shows/${slug}` : undefined
  const raw = ticketUrl?.trim()
  if (!raw) {
    return canonical
  }

  const hasScheme = /^[a-z][a-z0-9+.-]*:/i.test(raw)
  // Strip protocol-relative leading slashes so `//host/path` does not become
  // `https:////host/path`.
  const candidate = hasScheme ? raw : `https://${raw.replace(/^\/+/, '')}`

  try {
    const parsed = new URL(candidate)
    if (parsed.protocol === 'https:' || parsed.protocol === 'http:') {
      return candidate
    }
  } catch {
    // Not a parseable URL — fall through to the canonical page.
  }

  return canonical
}

/**
 * Generate MusicEvent schema for a show
 */
export function generateMusicEventSchema(show: {
  name?: string
  date: string
  description?: string
  is_cancelled?: boolean
  is_sold_out?: boolean
  /**
   * The show has already happened, so there is nothing left to offer.
   *
   * Caller-supplied rather than derived from `date` here, so this stays a pure
   * function of its input: its output is reproducible and its tests do not
   * depend on the clock.
   */
  is_past?: boolean
  venue?: {
    name: string
    slug?: string
    address?: string
    city?: string
    state?: string
    /** ISO country code. Defaults to US when the caller does not know. */
    country?: string
    timezone?: string | null
    zip_code?: string
  }
  artists?: Array<{
    name: string
    slug?: string
    is_headliner?: boolean
    socials?: Record<string, string | null | undefined>
  }>
  price?: number
  /** Where a ticket can actually be bought; normalized by `resolveOfferUrl`. */
  ticket_url?: string
  slug?: string
}): MusicEventSchema {
  const headliner = show.artists?.find(a => a.is_headliner)?.name || show.artists?.[0]?.name || 'Live Music'
  const eventName = show.name || `${headliner} at ${show.venue?.name || 'TBA'}`

  const schema: MusicEventSchema = {
    '@context': 'https://schema.org',
    '@type': 'MusicEvent',
    name: eventName,
    // Emit the venue-local start time with offset (PSY-986) so crawlers index
    // the real local time, not the bare UTC instant.
    startDate: show.venue
      ? toZonedISOString(
          show.date,
          resolveShowTimezone(show.venue.state, show.venue.timezone)
        )
      : show.date,
    eventStatus: show.is_cancelled
      ? 'https://schema.org/EventCancelled'
      : 'https://schema.org/EventScheduled',
    eventAttendanceMode: 'https://schema.org/OfflineEventAttendanceMode',
    location: {
      '@type': 'MusicVenue',
      name: show.venue?.name || 'TBA',
    },
    organizer: {
      '@type': 'Organization',
      name: 'Psychic Homily',
      url: SITE_URL,
    },
  }

  if (show.description) {
    schema.description = show.description
  }

  if (show.venue?.slug) {
    schema.location.url = `${SITE_URL}/venues/${show.venue.slug}`
  }

  if (show.venue?.address || show.venue?.city) {
    schema.location.address = {
      '@type': 'PostalAddress',
      streetAddress: show.venue.address,
      addressLocality: show.venue.city,
      addressRegion: show.venue.state,
      postalCode: show.venue.zip_code,
      // US only as the fallback for callers that cannot supply a country. The
      // scene list is not US-only — a non-US scene stamped `US` would be a
      // machine-readable false statement about a real place, repeated once per
      // show on the page.
      addressCountry: show.venue.country || 'US',
    }
  }

  if (show.artists && show.artists.length > 0) {
    schema.performer = show.artists.map(artist => {
      const performer: NonNullable<MusicEventSchema['performer']>[number] = {
        '@type': 'MusicGroup',
        name: artist.name,
      }

      if (artist.slug) {
        performer.url = `${SITE_URL}/artists/${artist.slug}`
      }

      if (artist.socials) {
        const socialLinks = Object.values(artist.socials).filter((v): v is string => !!v)
        if (socialLinks.length > 0) {
          performer.sameAs = socialLinks
        }
      }

      return performer
    })
  }

  // An offer describes what a reader can still get. A cancelled show has
  // nothing to sell and neither does one that already happened, so both drop
  // the offer entirely: emitting one produced `EventCancelled` next to
  // `availability: InStock` — the two halves of the same block contradicting
  // each other. No `ItemAvailability` value honestly means "cancelled" or
  // "over", so the claim is dropped rather than mapped onto a guess.
  //
  // Sold-out is NOT gated on price. `availability` is the only channel
  // schema.org has for it (there is no EventSoldOut status), so gating it on a
  // price the show may simply not have recorded would leave the sold-out badge
  // the page renders with no machine-readable counterpart. Price and currency
  // stay optional inside the offer — Google marks both Recommended, not
  // required, so a price-less Offer still validates.
  const hasPrice = show.price !== undefined && show.price !== null
  if (!show.is_cancelled && !show.is_past && (hasPrice || show.is_sold_out)) {
    schema.offers = {
      '@type': 'Offer',
      ...(hasPrice ? { price: show.price, priceCurrency: 'USD' } : {}),
      availability: show.is_sold_out
        ? 'https://schema.org/SoldOut'
        : 'https://schema.org/InStock',
      url: resolveOfferUrl(show.ticket_url, show.slug),
    }
  }

  if (show.slug) {
    schema.url = `${SITE_URL}/shows/${show.slug}`
    schema.image = [`${SITE_URL}/shows/${show.slug}/opengraph-image`]
  }

  return schema
}

/**
 * Generate BlogPosting schema for a blog post
 */
export function generateBlogPostingSchema(post: {
  title: string
  date: string
  description?: string
  slug: string
}): BlogPostingSchema {
  return {
    '@context': 'https://schema.org',
    '@type': 'BlogPosting',
    headline: post.title,
    datePublished: post.date,
    dateModified: post.date,
    description: post.description,
    author: {
      '@type': 'Organization',
      name: 'Psychic Homily',
    },
    url: `${SITE_URL}/blog/${post.slug}`,
  }
}

/**
 * Generate MusicVenue schema for a venue
 */
export function generateMusicVenueSchema(venue: {
  name: string
  address?: string
  city?: string
  state?: string
  zip_code?: string
  slug?: string
}): MusicVenueSchema {
  const schema: MusicVenueSchema = {
    '@context': 'https://schema.org',
    '@type': 'MusicVenue',
    name: venue.name,
  }

  if (venue.address || venue.city) {
    schema.address = {
      '@type': 'PostalAddress',
      streetAddress: venue.address,
      addressLocality: venue.city,
      addressRegion: venue.state,
      postalCode: venue.zip_code,
    }
  }

  if (venue.slug) {
    schema.url = `${SITE_URL}/venues/${venue.slug}`
  }

  return schema
}

/**
 * Generate MusicGroup schema for an artist
 */
export function generateMusicGroupSchema(artist: {
  name: string
  slug?: string
  city?: string | null
  state?: string | null
  social?: Record<string, string | null>
}): MusicGroupSchema {
  const schema: MusicGroupSchema = {
    '@context': 'https://schema.org',
    '@type': 'MusicGroup',
    name: artist.name,
  }

  if (artist.slug) {
    schema.url = `${SITE_URL}/artists/${artist.slug}`
  }

  // Add social links to sameAs
  if (artist.social) {
    const socialLinks: string[] = []
    for (const [, value] of Object.entries(artist.social)) {
      if (value) {
        socialLinks.push(value)
      }
    }
    if (socialLinks.length > 0) {
      schema.sameAs = socialLinks
    }
  }

  if (artist.city || artist.state) {
    schema.foundingLocation = {
      '@type': 'Place',
      address: {
        '@type': 'PostalAddress',
        addressLocality: artist.city || undefined,
        addressRegion: artist.state || undefined,
      },
    }
  }

  return schema
}

/**
 * Generate MusicRecording schema for a DJ set
 */
export function generateMusicRecordingSchema(mix: {
  title: string
  artist: string
  date: string
  slug: string
}): MusicRecordingSchema {
  return {
    '@context': 'https://schema.org',
    '@type': 'MusicRecording',
    name: mix.title,
    byArtist: {
      '@type': 'MusicGroup',
      name: mix.artist,
    },
    datePublished: mix.date,
    url: `${SITE_URL}/dj-sets/${mix.slug}`,
  }
}

/**
 * Generate ItemList schema for listing/discovery pages
 */
export function generateItemListSchema(items: {
  name?: string
  description?: string
  listItems: Array<{ url: string; name?: string }>
}): ItemListSchema {
  const schema: ItemListSchema = {
    '@context': 'https://schema.org',
    '@type': 'ItemList',
    numberOfItems: items.listItems.length,
    itemListElement: items.listItems.map((item, index) => ({
      '@type': 'ListItem' as const,
      position: index + 1,
      url: item.url,
      ...(item.name ? { name: item.name } : {}),
    })),
  }

  if (items.name) {
    schema.name = items.name
  }

  if (items.description) {
    schema.description = items.description
  }

  return schema
}

/**
 * Helper to render JSON-LD script tag
 * Use this in page components to embed structured data
 *
 * @example
 * <script
 *   type="application/ld+json"
 *   dangerouslySetInnerHTML={{ __html: JSON.stringify(schema) }}
 * />
 */
export function renderJsonLd(schema: object): string {
  return JSON.stringify(schema)
}

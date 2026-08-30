/**
 * JSON-LD Structured Data Helpers for SEO
 *
 * These helpers generate schema.org structured data that can be
 * embedded in pages to improve search engine understanding.
 */

import { resolveShowTimezone } from '@/lib/utils/formatters'
import { toZonedISOString } from '@/lib/utils/timeUtils'
import { SITE_DESCRIPTION, SITE_URL } from '@/lib/seo/siteMetadata'
import { resolveTicketVendor } from '@/lib/tickets/ticketVendors'

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
    /**
     * Who sells the ticket, when we can name them. Deliberately no `url` — see
     * the offers block in `generateMusicEventSchema`.
     */
    seller?: {
      '@type': 'Organization'
      name: string
    }
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
 * Name the ticket vendor behind a show's `ticket_url`, or `undefined` when it
 * is not one we recognize.
 *
 * Reads the shared vendor table (`lib/tickets/ticketVendors`), which is also
 * what the visible Buy Tickets link resolves against, so the company this page
 * names and the company it links to can never disagree. The URL itself is
 * never emitted here — only the name — so this reads a user-supplied field
 * purely to look up a constant.
 */
function ticketVendorName(ticketUrl: string | undefined): string | undefined {
  return resolveTicketVendor(ticketUrl)?.name
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
   * The show's advertised START INSTANT has passed, so there is nothing left to
   * sell. Both callers supply `hasShowStarted` from `lib/utils/showTiming`.
   *
   * This is NOT "the show's day is over". Do not feed it `isShowPast` from
   * that module, which draws the venue-local calendar-day boundary for listing
   * liveness. That boundary keeps an `InStock` offer standing through the show
   * itself, and for nearly a full day for a show starting after midnight,
   * which is a shortened form of the bug the offer gate exists to prevent.
   *
   * Caller-supplied rather than derived from `date` here, so this stays a pure
   * function of its input: its output is reproducible and its tests do not
   * depend on the clock.
   */
  has_started?: boolean
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
  /**
   * Read ONLY to name the vendor in `offers.seller`. The URL itself is never
   * emitted — see the offers block below.
   */
  ticket_url?: string
  /** The flyer, when the show has one. Emitted alongside the generated card. */
  image_url?: string | null
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
  //
  // There is deliberately NO `offers.url`. Google documents it as Recommended,
  // not required, and the only thing omitting it costs is the "ticket purchase
  // option" placement — price display and sold-out badging both survive
  // without it. Neither available value is honest: the vendor's own URL hands
  // the sale to a company this site has no referral arrangement with, and a
  // self-referencing URL fails Google's own bar for the field, a "landing page
  // that clearly and predominantly provides the opportunity to buy". So the
  // offer says only what it can back up — the price, whether it is sold out,
  // and who sells it.
  //
  // The gate is price-or-sold-out for the same reason: with no url, an offer
  // carrying neither conveys nothing at all.
  const hasPrice = show.price !== undefined && show.price !== null
  if (!show.is_cancelled && !show.has_started && (hasPrice || show.is_sold_out)) {
    const seller = ticketVendorName(show.ticket_url)
    schema.offers = {
      '@type': 'Offer',
      ...(hasPrice ? { price: show.price, priceCurrency: 'USD' } : {}),
      availability: show.is_sold_out
        ? 'https://schema.org/SoldOut'
        : 'https://schema.org/InStock',
      ...(seller ? { seller: { '@type': 'Organization' as const, name: seller } } : {}),
    }
  }

  if (show.slug) {
    schema.url = `${SITE_URL}/shows/${show.slug}`
  }

  // Both images, generated card FIRST.
  //
  // Google treats the array as ranked preference, and the card is the one we
  // control: it is always 1200×630, always contains the date, venue and bill as
  // text, and always exists. The flyer is the richer artefact but an unknown
  // quantity — arbitrary aspect ratio, possibly a dead link, and cropped to
  // whatever ratio a given surface wants. Listing it second offers it without
  // betting the result on it.
  //
  // The flyer is emitted even when the OG route could not render it: the two
  // are independent claims, and a consumer fetching the URL directly is not
  // affected by whatever made the card fall back to text.
  const flyer = absoluteHttpUrl(show.image_url)
  const images = [
    ...(show.slug ? [`${SITE_URL}/shows/${show.slug}/opengraph-image`] : []),
    ...(flyer ? [flyer] : []),
  ]
  if (images.length > 0) {
    schema.image = images
  }

  return schema
}

/**
 * The NORMALISED absolute http(s) URL, or null.
 *
 * Returns `href` rather than the input because URL parsing is far more lenient
 * than the value it produces: `"  https://x/a "` and `"https://x/a\nb"` both
 * parse, and the backend stores `image_url` untrimmed, so the padded form
 * genuinely reaches this builder. Emitting the raw string would put whitespace
 * and control characters inside a machine-readable image claim — the broken
 * output this check exists to prevent, just in a subtler form than a relative
 * path.
 */
function absoluteHttpUrl(value: string | null | undefined): string | null {
  if (!value) return null
  try {
    const url = new URL(value)
    return url.protocol === 'http:' || url.protocol === 'https:' ? url.href : null
  } catch {
    return null
  }
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

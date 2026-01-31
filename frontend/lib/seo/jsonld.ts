/**
 * JSON-LD Structured Data Helpers for SEO
 *
 * These helpers generate schema.org structured data that can be
 * embedded in pages to improve search engine understanding.
 */

export interface OrganizationSchema {
  '@context': 'https://schema.org'
  '@type': 'Organization'
  name: string
  url: string
  description?: string
  sameAs?: string[]
}

export interface MusicEventSchema {
  '@context': 'https://schema.org'
  '@type': 'MusicEvent'
  name: string
  startDate: string
  location: {
    '@type': 'MusicVenue'
    name: string
    address?: {
      '@type': 'PostalAddress'
      streetAddress?: string
      addressLocality?: string
      addressRegion?: string
      postalCode?: string
    }
  }
  performer?: Array<{
    '@type': 'MusicGroup'
    name: string
  }>
  offers?: {
    '@type': 'Offer'
    price?: string
    priceCurrency?: string
    availability?: string
    url?: string
  }
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

/**
 * Generate Organization schema for the site
 */
export function generateOrganizationSchema(): OrganizationSchema {
  return {
    '@context': 'https://schema.org',
    '@type': 'Organization',
    name: 'Psychic Homily',
    url: 'https://psychichomily.com',
    description: 'Discover upcoming live music shows, blog posts, and DJ sets from the Arizona music scene.',
  }
}

/**
 * Generate MusicEvent schema for a show
 */
export function generateMusicEventSchema(show: {
  name?: string
  date: string
  venue?: {
    name: string
    address?: string
    city?: string
    state?: string
    zip_code?: string
  }
  artists?: Array<{ name: string; is_headliner?: boolean }>
  ticket_url?: string
  price?: string
  slug?: string
}): MusicEventSchema {
  const headliner = show.artists?.find(a => a.is_headliner)?.name || show.artists?.[0]?.name || 'Live Music'
  const eventName = show.name || `${headliner} at ${show.venue?.name || 'TBA'}`

  const schema: MusicEventSchema = {
    '@context': 'https://schema.org',
    '@type': 'MusicEvent',
    name: eventName,
    startDate: show.date,
    location: {
      '@type': 'MusicVenue',
      name: show.venue?.name || 'TBA',
    },
  }

  if (show.venue?.address || show.venue?.city) {
    schema.location.address = {
      '@type': 'PostalAddress',
      streetAddress: show.venue.address,
      addressLocality: show.venue.city,
      addressRegion: show.venue.state,
      postalCode: show.venue.zip_code,
    }
  }

  if (show.artists && show.artists.length > 0) {
    schema.performer = show.artists.map(artist => ({
      '@type': 'MusicGroup',
      name: artist.name,
    }))
  }

  if (show.ticket_url) {
    schema.offers = {
      '@type': 'Offer',
      availability: 'https://schema.org/InStock',
      url: show.ticket_url,
    }
    if (show.price) {
      schema.offers.price = show.price
      schema.offers.priceCurrency = 'USD'
    }
  }

  if (show.slug) {
    schema.url = `https://psychichomily.com/shows/${show.slug}`
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
    url: `https://psychichomily.com/blog/${post.slug}`,
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
    schema.url = `https://psychichomily.com/venues/${venue.slug}`
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

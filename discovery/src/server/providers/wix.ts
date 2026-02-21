import type { DiscoveredEvent, PreviewEvent, DiscoveryProvider, OnScrapeProgress } from './types'
import { cleanArtistName } from './artistUtils'

// Venue configurations for Wix provider
const VENUES: Record<string, { name: string; url: string; sitemapUrl: string }> = {
  'celebrity-theatre': {
    name: 'Celebrity Theatre',
    url: 'https://www.celebritytheatre.com',
    sitemapUrl: 'https://www.celebritytheatre.com/event-pages-sitemap.xml',
  },
}

// JSON-LD Event shape (Wix uses @type "Event", not "MusicEvent")
interface WixJsonLdEvent {
  '@type'?: string
  name?: string
  startDate?: string
  endDate?: string
  url?: string
  image?: string | { '@type'?: string; url?: string } | Array<string | { '@type'?: string; url?: string }>
  eventStatus?: string
  eventAttendanceMode?: string
  location?: {
    '@type'?: string
    name?: string
    address?: {
      streetAddress?: string
      addressLocality?: string
      addressRegion?: string
    }
  }
  performer?: Array<{ name?: string }> | { name?: string }
  offers?: { url?: string; price?: number | string; availability?: string } | Array<{ url?: string; price?: number | string; availability?: string }>
  description?: string
}

/**
 * Fetch sitemap XML and extract event page URLs.
 * Filters to paths matching /events/{slug} pattern.
 */
async function fetchSitemapUrls(sitemapUrl: string): Promise<string[]> {
  const response = await fetch(sitemapUrl, {
    headers: { 'User-Agent': 'Mozilla/5.0 (compatible; PsychicHomily/1.0)' },
  })

  if (!response.ok) {
    throw new Error(`Failed to fetch sitemap ${sitemapUrl}: ${response.status} ${response.statusText}`)
  }

  const xml = await response.text()
  const urls: string[] = []
  const locRegex = /<loc>\s*(.*?)\s*<\/loc>/gi
  let match: RegExpExecArray | null

  while ((match = locRegex.exec(xml)) !== null) {
    const url = match[1]
    // Only include event detail pages (e.g., /events/some-event-slug)
    if (/\/events\/[^/]+$/.test(url)) {
      urls.push(url)
    }
  }

  return urls
}

/**
 * Extract JSON-LD Event data from an HTML page.
 * Wix sites use @type "Event" (not "MusicEvent").
 */
function parseJsonLdEvents(html: string): WixJsonLdEvent[] {
  const events: WixJsonLdEvent[] = []
  const regex = /<script\s+type=["']application\/ld\+json["'][^>]*>([\s\S]*?)<\/script>/gi
  let match: RegExpExecArray | null

  while ((match = regex.exec(html)) !== null) {
    try {
      const data = JSON.parse(match[1])
      const items = Array.isArray(data) ? data : [data]
      for (const item of items) {
        if (item['@type'] === 'Event' || item['@type'] === 'MusicEvent') {
          events.push(item)
        }
      }
    } catch {
      // Skip malformed JSON-LD blocks
    }
  }

  return events
}

/**
 * Fetch a single page and extract JSON-LD event data.
 * Returns null if no event JSON-LD is found.
 */
async function fetchEventJsonLd(url: string): Promise<{ url: string; event: WixJsonLdEvent } | null> {
  try {
    const response = await fetch(url, {
      headers: { 'User-Agent': 'Mozilla/5.0 (compatible; PsychicHomily/1.0)' },
    })

    if (!response.ok) return null

    const html = await response.text()
    const events = parseJsonLdEvents(html)
    if (events.length === 0) return null

    return { url, event: events[0] }
  } catch {
    return null
  }
}

/**
 * Run async tasks with a concurrency limit.
 */
async function fetchConcurrent<T>(
  items: string[],
  fn: (item: string) => Promise<T | null>,
  limit = 10,
): Promise<Array<T>> {
  const results: T[] = []
  let index = 0

  async function worker() {
    while (index < items.length) {
      const i = index++
      const result = await fn(items[i])
      if (result) results.push(result)
    }
  }

  const workers = Array.from({ length: Math.min(limit, items.length) }, () => worker())
  await Promise.all(workers)
  return results
}

/**
 * Extract date in YYYY-MM-DD format from an ISO 8601 startDate.
 */
function extractDate(startDate: string | undefined): string {
  if (!startDate) return ''
  return startDate.slice(0, 10)
}

/**
 * Extract time (e.g. "8:00 PM") from an ISO 8601 datetime string.
 * Parses time directly from the string to avoid timezone conversion.
 */
function extractTime(isoDate: string | undefined): string | undefined {
  if (!isoDate) return undefined
  const match = isoDate.match(/T(\d{2}):(\d{2})/)
  if (!match) return undefined

  const hours = parseInt(match[1], 10)
  const minutes = parseInt(match[2], 10)
  const ampm = hours >= 12 ? 'PM' : 'AM'
  const h = hours % 12 || 12
  const m = minutes.toString().padStart(2, '0')
  return `${h}:${m} ${ampm}`
}

/**
 * Extract image URL from Wix JSON-LD image field.
 * Handles string, ImageObject with nested .url, or arrays of either.
 */
function extractImageUrl(image: WixJsonLdEvent['image']): string | undefined {
  if (!image) return undefined

  if (typeof image === 'string') return image

  if (Array.isArray(image)) {
    for (const item of image) {
      if (typeof item === 'string') return item
      if (item && typeof item === 'object' && item.url) return item.url
    }
    return undefined
  }

  if (typeof image === 'object' && image.url) return image.url

  return undefined
}

/**
 * Check if the event is cancelled based on eventStatus.
 */
function isCancelled(event: WixJsonLdEvent): boolean {
  if (!event.eventStatus) return false
  return event.eventStatus.includes('EventCancelled')
}

/**
 * Extract artist names from the event title.
 * Strips tour suffixes like " - The Something Tour".
 */
function extractArtists(event: WixJsonLdEvent): string[] {
  // Try performer field first
  if (event.performer) {
    const performers = Array.isArray(event.performer) ? event.performer : [event.performer]
    const names = performers.map(p => p.name).filter((n): n is string => !!n).map(cleanArtistName)
    if (names.length > 0) return names
  }

  // Parse from title: strip tour name suffixes
  const title = event.name || ''
  const cleaned = cleanArtistName(title.replace(/\s*[-–—]\s*(?:the\s+)?[^-–—]*tour.*$/i, '').trim())
  return cleaned ? [cleaned] : [title]
}

/**
 * Extract event slug from a full URL to use as a stable ID.
 * e.g. https://www.celebritytheatre.com/events/some-show → "some-show"
 */
function extractSlugFromUrl(url: string): string {
  const match = url.match(/\/events\/([^/?#]+)/)
  return match ? match[1] : url
}

/**
 * Check if an event date is in the future (today or later).
 */
function isFutureEvent(startDate: string | undefined): boolean {
  if (!startDate) return false
  const eventDate = new Date(startDate)
  const today = new Date()
  today.setHours(0, 0, 0, 0)
  return eventDate >= today
}

/**
 * Extract ticket URL from event url or offers.
 */
function extractTicketUrl(event: WixJsonLdEvent): string | undefined {
  if (event.offers) {
    const offer = Array.isArray(event.offers) ? event.offers[0] : event.offers
    if (offer?.url) return offer.url
  }
  return event.url || undefined
}

/**
 * Extract price from offers field.
 */
function extractOfferPrice(event: WixJsonLdEvent): string | undefined {
  if (!event.offers) return undefined
  const offer = Array.isArray(event.offers) ? event.offers[0] : event.offers
  if (offer?.price !== undefined && offer.price !== null) {
    const price = Number(offer.price)
    if (price === 0) return 'Free'
    if (!isNaN(price)) return `$${price}`
  }
  return undefined
}

/**
 * Check if the event is sold out based on offers availability.
 */
function isSoldOut(event: WixJsonLdEvent): boolean {
  if (!event.offers) return false
  const offers = Array.isArray(event.offers) ? event.offers : [event.offers]
  return offers.some(o => o.availability?.includes('SoldOut') === true)
}

// Wix provider implementation
export const wixProvider: DiscoveryProvider = {
  async preview(venueSlug: string): Promise<PreviewEvent[]> {
    const venue = VENUES[venueSlug]
    if (!venue) {
      throw new Error(`Unknown venue: ${venueSlug}`)
    }

    console.log(`[wix] Previewing events from ${venue.name} via sitemap...`)

    // Fetch sitemap to get all event page URLs
    const urls = await fetchSitemapUrls(venue.sitemapUrl)
    console.log(`[wix] Found ${urls.length} event URLs in sitemap`)

    // Fetch each page concurrently and extract JSON-LD
    const results = await fetchConcurrent(urls, fetchEventJsonLd, 10)
    console.log(`[wix] Fetched JSON-LD from ${results.length} pages`)

    // Filter to future events and map to preview format
    const previewEvents: PreviewEvent[] = results
      .filter(r => isFutureEvent(r.event.startDate))
      .map(r => ({
        id: extractSlugFromUrl(r.url),
        title: r.event.name || 'Unknown Event',
        date: extractDate(r.event.startDate),
        venue: r.event.location?.name || venue.name,
      }))
      .sort((a, b) => a.date.localeCompare(b.date))

    console.log(`[wix] ${previewEvents.length} upcoming events`)
    return previewEvents
  },

  async scrape(venueSlug: string, eventIds: string[], onProgress?: OnScrapeProgress): Promise<DiscoveredEvent[]> {
    const venue = VENUES[venueSlug]
    if (!venue) {
      throw new Error(`Unknown venue: ${venueSlug}`)
    }

    console.log(`[wix] Scraping ${eventIds.length} events from ${venue.name}...`)

    // Build URLs from event slugs
    const urls = eventIds.map(slug => `${venue.url}/events/${slug}`)

    // Fetch all pages concurrently
    const results = await fetchConcurrent(urls, fetchEventJsonLd, 10)

    const scrapedEvents: DiscoveredEvent[] = results.map((r, i) => {
      onProgress?.({
        current: i + 1,
        total: results.length,
        eventTitle: (r.event.name || 'Unknown Event').slice(0, 60),
        phase: 'assembling',
      })

      const event = r.event
      const slug = extractSlugFromUrl(r.url)
      const cancelled = isCancelled(event)
      const soldOut = isSoldOut(event)

      return {
        id: slug,
        title: event.name || 'Unknown Event',
        date: extractDate(event.startDate),
        venue: event.location?.name || venue.name,
        venueSlug,
        imageUrl: extractImageUrl(event.image),
        doorsTime: undefined,
        showTime: extractTime(event.startDate),
        ticketUrl: extractTicketUrl(event),
        artists: extractArtists(event),
        scrapedAt: new Date().toISOString(),
        price: extractOfferPrice(event),
        isSoldOut: soldOut || undefined,
        isCancelled: cancelled || undefined,
      }
    })

    console.log(`[wix] Scraped ${scrapedEvents.length} events`)
    return scrapedEvents
  },
}

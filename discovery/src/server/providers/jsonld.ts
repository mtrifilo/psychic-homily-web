import type { DiscoveredEvent, PreviewEvent, DiscoveryProvider } from './types'

// Venue configurations for JSON-LD provider
const VENUES: Record<string, { name: string; url: string }> = {
  'the-van-buren': {
    name: 'The Van Buren',
    url: 'https://thevanburenphx.com/shows',
  },
  'arizona-financial-theatre': {
    name: 'Arizona Financial Theatre',
    url: 'https://www.arizonafinancialtheatre.com/shows',
  },
}

// JSON-LD MusicEvent shape (subset of schema.org/MusicEvent)
interface JsonLdMusicEvent {
  '@context'?: string
  '@type'?: string
  name?: string
  startDate?: string
  url?: string
  image?: string
  eventStatus?: string
  location?: {
    '@type'?: string
    name?: string
  }
  doorTime?: string
  performer?: Array<{ name?: string }> | { name?: string }
  offers?: { url?: string; price?: number | string; priceCurrency?: string; availability?: string } | Array<{ url?: string; price?: number | string; priceCurrency?: string; availability?: string }>
  typicalAgeRange?: string
}

/**
 * Extract all JSON-LD blocks from HTML and filter for MusicEvent types.
 */
function parseMusicEvents(html: string): JsonLdMusicEvent[] {
  const events: JsonLdMusicEvent[] = []
  const regex = /<script\s+type=["']application\/ld\+json["'][^>]*>([\s\S]*?)<\/script>/gi
  let match: RegExpExecArray | null

  while ((match = regex.exec(html)) !== null) {
    try {
      const data = JSON.parse(match[1])
      // Handle single objects or arrays
      const items = Array.isArray(data) ? data : [data]
      for (const item of items) {
        if (item['@type'] === 'MusicEvent') {
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
 * Extract an event ID from a Ticketmaster URL, or generate one from title+date.
 */
function extractEventId(event: JsonLdMusicEvent): string {
  // Try to extract Ticketmaster event ID from URL
  // e.g. https://www.ticketmaster.com/.../event/19006365E70B9B37
  if (event.url) {
    const tmMatch = event.url.match(/\/event\/([A-Za-z0-9]+)$/)
    if (tmMatch) {
      return tmMatch[1]
    }
  }

  // Fallback: hash from title + date
  const raw = `${event.name || ''}|${event.startDate || ''}`
  let hash = 0
  for (let i = 0; i < raw.length; i++) {
    hash = ((hash << 5) - hash + raw.charCodeAt(i)) | 0
  }
  return `jsonld-${Math.abs(hash).toString(36)}`
}

/**
 * Extract date in YYYY-MM-DD format from an ISO 8601 startDate.
 */
function extractDate(startDate: string | undefined): string {
  if (!startDate) return ''
  // startDate format: "2026-02-07T10:00:00-07:00"
  return startDate.slice(0, 10)
}

/**
 * Extract time (e.g. "8:00 PM") from an ISO 8601 datetime string.
 * Parses the time directly from the string to avoid timezone conversion —
 * the time before the offset in ISO 8601 is already the local event time.
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
 * Extract artist names from performer field, or parse from the event title.
 */
function extractArtists(event: JsonLdMusicEvent): string[] {
  // Try performer field first
  if (event.performer) {
    const performers = Array.isArray(event.performer) ? event.performer : [event.performer]
    const names = performers.map(p => p.name).filter((n): n is string => !!n)
    if (names.length > 0) return names
  }

  // Parse from title: strip tour name suffixes like " - The Something Tour"
  const title = event.name || ''
  const cleaned = title.replace(/\s*[-–—]\s*(?:the\s+)?[^-–—]*tour.*$/i, '').trim()
  return cleaned ? [cleaned] : [title]
}

/**
 * Extract ticket URL from event url or offers.
 */
function extractTicketUrl(event: JsonLdMusicEvent): string | undefined {
  if (event.url) return event.url
  if (event.offers) {
    const offer = Array.isArray(event.offers) ? event.offers[0] : event.offers
    return offer?.url
  }
  return undefined
}

/**
 * Extract price from offers field.
 */
function extractOfferPrice(event: JsonLdMusicEvent): string | undefined {
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
 * Extract age restriction from typicalAgeRange or event name.
 */
function extractAge(event: JsonLdMusicEvent): string | undefined {
  // Try typicalAgeRange field first (e.g., "18-" means 18+)
  if (event.typicalAgeRange) {
    const match = event.typicalAgeRange.match(/(\d{1,2})/)
    if (match) return `${match[1]}+`
  }
  // Fallback: parse from event name, e.g. "(18+)" or "(All Ages)"
  const name = event.name || ''
  if (/\ball\s*ages?\b/i.test(name)) return 'All Ages'
  const ageMatch = name.match(/\((\d{1,2})\+\)/)
  if (ageMatch) return `${ageMatch[1]}+`
  return undefined
}

/**
 * Check if the event is cancelled based on eventStatus.
 */
function isCancelled(event: JsonLdMusicEvent): boolean {
  if (!event.eventStatus) return false
  return event.eventStatus.includes('EventCancelled')
}

/**
 * Check if the event is sold out based on offers availability.
 */
function isSoldOut(event: JsonLdMusicEvent): boolean {
  if (!event.offers) return false
  const offers = Array.isArray(event.offers) ? event.offers : [event.offers]
  return offers.some(o => o.availability?.includes('SoldOut') === true)
}

// JSON-LD provider implementation
export const jsonldProvider: DiscoveryProvider = {
  async preview(venueSlug: string): Promise<PreviewEvent[]> {
    const venue = VENUES[venueSlug]
    if (!venue) {
      throw new Error(`Unknown venue: ${venueSlug}`)
    }

    console.log(`[jsonld] Previewing events from ${venue.name}...`)

    const response = await fetch(venue.url, {
      headers: {
        'User-Agent': 'Mozilla/5.0 (compatible; PsychicHomily/1.0)',
      },
    })

    if (!response.ok) {
      throw new Error(`Failed to fetch ${venue.url}: ${response.status} ${response.statusText}`)
    }

    const html = await response.text()
    const events = parseMusicEvents(html)

    const previewEvents: PreviewEvent[] = events.map((event) => ({
      id: extractEventId(event),
      title: event.name || 'Unknown Event',
      date: extractDate(event.startDate),
      venue: event.location?.name || venue.name,
    }))

    console.log(`[jsonld] Found ${previewEvents.length} events`)
    return previewEvents
  },

  async scrape(venueSlug: string, eventIds: string[]): Promise<DiscoveredEvent[]> {
    const venue = VENUES[venueSlug]
    if (!venue) {
      throw new Error(`Unknown venue: ${venueSlug}`)
    }

    console.log(`[jsonld] Scraping ${eventIds.length} events from ${venue.name}...`)

    const response = await fetch(venue.url, {
      headers: {
        'User-Agent': 'Mozilla/5.0 (compatible; PsychicHomily/1.0)',
      },
    })

    if (!response.ok) {
      throw new Error(`Failed to fetch ${venue.url}: ${response.status} ${response.statusText}`)
    }

    const html = await response.text()
    const allEvents = parseMusicEvents(html)

    // Build a map of all events by ID for filtering
    const eventMap = new Map<string, JsonLdMusicEvent>()
    for (const event of allEvents) {
      eventMap.set(extractEventId(event), event)
    }

    const eventIdSet = new Set(eventIds)
    const scrapedEvents: DiscoveredEvent[] = []

    for (const [id, event] of eventMap) {
      if (!eventIdSet.has(id)) continue

      const cancelled = isCancelled(event)
      const soldOut = isSoldOut(event)

      scrapedEvents.push({
        id,
        title: event.name || 'Unknown Event',
        date: extractDate(event.startDate),
        venue: event.location?.name || venue.name,
        venueSlug,
        imageUrl: event.image || undefined,
        doorsTime: extractTime(event.doorTime),
        showTime: extractTime(event.startDate),
        ticketUrl: extractTicketUrl(event),
        artists: extractArtists(event),
        scrapedAt: new Date().toISOString(),
        price: extractOfferPrice(event),
        ageRestriction: extractAge(event),
        isSoldOut: soldOut || undefined,
        isCancelled: cancelled || undefined,
      })
    }

    console.log(`[jsonld] Scraped ${scrapedEvents.length} events`)
    return scrapedEvents
  },
}

import { chromium } from 'playwright'
import type { DiscoveredEvent, PreviewEvent, DiscoveryProvider, OnScrapeProgress } from './types'
import { cleanArtistName } from './artistUtils'

// Venue configurations for SeeTickets provider
const VENUES: Record<string, { name: string; url: string }> = {
  'the-rebel-lounge': {
    name: 'The Rebel Lounge',
    url: 'https://therebellounge.com/events/',
  },
}

/**
 * Extract the numeric SeeTickets event ID from a ticket URL.
 * e.g. https://wl.seetickets.us/event/a-wilhelm-scream/670245?afflky=... → "670245"
 */
function extractEventId(url: string): string {
  const match = url.match(/\/(\d+)(?:\?|$)/)
  return match ? match[1] : url
}

/**
 * Parse a date string like "Thu Feb 19" into YYYY-MM-DD format.
 * Assumes the event is in the current or next year.
 */
function parseDate(dateText: string): string {
  // Remove day-of-week prefix (e.g. "Thu ")
  const cleaned = dateText.replace(/^[A-Za-z]+\s+/, '')
  const now = new Date()
  const currentYear = now.getFullYear()

  // Try current year first, then next year
  for (const year of [currentYear, currentYear + 1]) {
    const attempt = new Date(`${cleaned} ${year}`)
    if (!isNaN(attempt.getTime())) {
      // If the date is more than 2 months in the past, use next year
      if (year === currentYear && attempt < new Date(now.getFullYear(), now.getMonth() - 2, 1)) {
        continue
      }
      return attempt.toISOString().slice(0, 10)
    }
  }

  return ''
}

/**
 * Normalize time string to "H:MM PM" format.
 */
function formatTime(raw: string): string {
  const match = raw.match(/(\d{1,2}):(\d{2})\s*([AP]M)/i)
  if (!match) return raw.trim()
  const h = parseInt(match[1], 10)
  return `${h}:${match[2]} ${match[3].toUpperCase()}`
}

/**
 * Parse artists from the headliner name and supporting-talent text.
 * Headliner comes from p.headliners (already proper case).
 * Openers from p.supporting-talent ("with X, Y, Z").
 */
function parseArtists(headliner: string, supportingTalent?: string): string[] {
  const artists: string[] = []
  if (headliner) {
    // Split co-headliners (e.g. "Annika Wells, Rachel Bochner")
    const headliners = headliner.split(/\s*,\s*/).map(s => cleanArtistName(s.trim())).filter(Boolean)
    artists.push(...headliners)
  }

  if (supportingTalent) {
    // Strip leading "with " prefix
    const cleaned = supportingTalent.replace(/^with\s+/i, '').trim()
    if (cleaned) {
      const parts = cleaned
        .split(/\s*,\s*/)
        .flatMap(part => part.split(/\s+and\s+/i))
        .map(s => cleanArtistName(s.trim()))
        .filter(s => s.length > 0 && !/^special\s+guests?$/i.test(s))
      artists.push(...parts)
    }
  }

  return artists
}

// SeeTickets provider implementation
export const seeticketsProvider: DiscoveryProvider = {
  async preview(venueSlug: string): Promise<PreviewEvent[]> {
    const venue = VENUES[venueSlug]
    if (!venue) {
      throw new Error(`Unknown venue: ${venueSlug}`)
    }

    console.log(`[seetickets] Previewing events from ${venue.name}...`)

    const browser = await chromium.launch({ headless: true })
    const page = await browser.newPage()

    try {
      await page.goto(venue.url, { waitUntil: 'networkidle', timeout: 60000 })

      // Wait for SeeTickets widget to render
      await page.waitForSelector('.seetickets-list-event-container', { state: 'attached', timeout: 30000 })

      const events = await page.$$eval('.seetickets-list-event-container', (containers) => {
        return containers.map((container) => {
          const titleLink = container.querySelector('p.title a') as HTMLAnchorElement | null
          const title = titleLink?.textContent?.trim() || ''
          const ticketUrl = titleLink?.href || ''
          const headliner = container.querySelector('p.headliners')?.textContent?.trim() || ''
          const dateText = container.querySelector('p.date')?.textContent?.trim() || ''
          return { title, headliner, ticketUrl, dateText }
        })
      })

      const previewEvents: PreviewEvent[] = events
        .filter(e => e.ticketUrl)
        .map(e => ({
          id: extractEventId(e.ticketUrl),
          title: e.headliner || e.title,
          date: parseDate(e.dateText),
          venue: venue.name,
        }))
        .filter(e => e.date !== '')

      console.log(`[seetickets] Found ${previewEvents.length} events`)
      return previewEvents
    } finally {
      await browser.close()
    }
  },

  async scrape(venueSlug: string, eventIds: string[], onProgress?: OnScrapeProgress): Promise<DiscoveredEvent[]> {
    const venue = VENUES[venueSlug]
    if (!venue) {
      throw new Error(`Unknown venue: ${venueSlug}`)
    }

    console.log(`[seetickets] Scraping ${eventIds.length} events from ${venue.name}...`)

    const browser = await chromium.launch({ headless: true })
    const page = await browser.newPage()

    try {
      await page.goto(venue.url, { waitUntil: 'networkidle', timeout: 60000 })
      await page.waitForSelector('.seetickets-list-event-container', { state: 'attached', timeout: 30000 })

      const eventIdSet = new Set(eventIds)

      // Extract all event data from the page using semantic selectors
      const rawEvents = await page.$$eval('.seetickets-list-event-container', (containers) => {
        return containers.map((container) => {
          const titleLink = container.querySelector('p.title a') as HTMLAnchorElement | null
          const title = titleLink?.textContent?.trim() || ''
          const ticketUrl = titleLink?.href || ''
          const headliner = container.querySelector('p.headliners')?.textContent?.trim() || ''
          const supportingTalent = container.querySelector('p.supporting-talent')?.textContent?.trim() || ''
          const dateText = container.querySelector('p.date')?.textContent?.trim() || ''
          const timeText = container.querySelector('p.doortime-showtime')?.textContent?.trim() || ''
          const ages = container.querySelector('span.ages')?.textContent?.trim() || ''
          const price = container.querySelector('span.price')?.textContent?.trim() || ''
          const img = container.querySelector('img') as HTMLImageElement | null
          const imageUrl = img?.src || ''
          const buyBlock = container.querySelector('.buy-and-share-block')
          const isSoldOut = /sold\s*out/i.test(buyBlock?.textContent || '')

          return { title, headliner, ticketUrl, supportingTalent, dateText, timeText, ages, price, imageUrl, isSoldOut }
        })
      })

      // Filter to selected events and build DiscoveredEvent objects
      const scrapedEvents: DiscoveredEvent[] = []
      let progressCount = 0

      for (const raw of rawEvents) {
        const id = extractEventId(raw.ticketUrl)
        if (!eventIdSet.has(id)) continue

        progressCount++
        onProgress?.({
          current: progressCount,
          total: eventIds.length,
          eventTitle: (raw.headliner || raw.title).slice(0, 60),
          phase: 'processing',
        })

        const date = parseDate(raw.dateText)
        if (!date) continue

        // Parse doors/show times from the time text
        const doorsMatch = raw.timeText.match(/Doors\s+at\s+(\d{1,2}:\d{2}\s*[AP]M)/i)
        const showMatch = raw.timeText.match(/Show\s+at\s+(\d{1,2}:\d{2}\s*[AP]M)/i)

        // Use headliner (properly cased) as the display title
        const displayTitle = raw.headliner || raw.title
        const artists = parseArtists(raw.headliner || raw.title, raw.supportingTalent)

        // Parse price — use the raw price text (e.g. "$25.00")
        let price: string | undefined
        if (raw.price) {
          price = raw.price.startsWith('$') ? raw.price : `$${raw.price}`
        }

        // Parse age restriction
        let ageRestriction: string | undefined
        if (raw.ages) {
          if (/all\s*ages?/i.test(raw.ages)) {
            ageRestriction = 'All Ages'
          } else {
            const ageMatch = raw.ages.match(/(\d{1,2})\+/)
            ageRestriction = ageMatch ? `${ageMatch[1]}+` : raw.ages
          }
        }

        scrapedEvents.push({
          id,
          title: displayTitle,
          date,
          venue: venue.name,
          venueSlug,
          imageUrl: raw.imageUrl || undefined,
          doorsTime: doorsMatch ? formatTime(doorsMatch[1]) : undefined,
          showTime: showMatch ? formatTime(showMatch[1]) : undefined,
          ticketUrl: raw.ticketUrl || undefined,
          artists,
          scrapedAt: new Date().toISOString(),
          price,
          ageRestriction,
          isSoldOut: raw.isSoldOut || undefined,
        })
      }

      console.log(`[seetickets] Scraped ${scrapedEvents.length} events`)
      return scrapedEvents
    } finally {
      await browser.close()
    }
  },
}

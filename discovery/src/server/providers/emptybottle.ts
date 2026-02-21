import { chromium } from 'playwright'
import type { DiscoveredEvent, PreviewEvent, DiscoveryProvider, OnScrapeProgress } from './types'
import { cleanArtistName } from './artistUtils'

// Venue configurations for Empty Bottle provider
const VENUES: Record<string, { name: string; url: string }> = {
  'empty-bottle': {
    name: 'Empty Bottle',
    url: 'https://www.emptybottle.com/',
  },
}

/**
 * Extract TicketWeb event ID from a buy-button URL.
 * e.g. https://www.ticketweb.com/event/dana-clickbait-cel-empty-bottle-tickets/14054484 → "14054484"
 */
function extractEventId(url: string): string {
  const match = url.match(/\/(\d+)(?:\?|$)/)
  return match ? match[1] : url
}

/**
 * Parse a date like "Thu February 19" into YYYY-MM-DD format.
 */
function parseDate(dateText: string): string {
  // Remove day-of-week prefix
  const cleaned = dateText.replace(/^[A-Za-z]+\s+/, '')
  const now = new Date()
  const currentYear = now.getFullYear()

  for (const year of [currentYear, currentYear + 1]) {
    const attempt = new Date(`${cleaned} ${year}`)
    if (!isNaN(attempt.getTime())) {
      if (year === currentYear && attempt < new Date(now.getFullYear(), now.getMonth() - 2, 1)) {
        continue
      }
      return attempt.toISOString().slice(0, 10)
    }
  }

  return ''
}

/**
 * Normalize time string like "9:00PM" to "9:00 PM".
 */
function formatTime(raw: string): string {
  const match = raw.match(/(\d{1,2}:\d{2})\s*([AP]M)/i)
  if (!match) return raw.trim()
  return `${match[1]} ${match[2].toUpperCase()}`
}

/**
 * Clean up artist names from the performing list.
 *
 * The widget's `<li>` elements have some quirks:
 * - First li may include event/series context: "Music Frozen Dancing with Los Thuthanaka"
 * - First li may be a truncated "FREE MONDAY w" (broken at " / " split)
 * - First li may have "*SOLD OUT*" prefix
 */
function cleanArtists(rawArtists: string[]): string[] {
  const artists: string[] = []

  for (let i = 0; i < rawArtists.length; i++) {
    let name = rawArtists[i].trim()
    if (!name) continue

    // Strip *SOLD OUT* / *CANCELLED* markers
    name = name.replace(/^\*(?:SOLD OUT|CANCELLED)\*\s*/i, '').trim()

    if (i === 0) {
      // Skip truncated series labels like "FREE MONDAY w" or "FREE TUESDAY w"
      if (/^FREE\s+\w+\s+w$/i.test(name)) continue

      // Extract artist from "Event Series with Artist Name" pattern
      const withMatch = name.match(/^.+?\swith\s+(.+)$/i)
      if (withMatch) {
        name = withMatch[1].trim()
      }
    }

    // Skip empty or pure event labels
    if (!name || /^FREE\s/i.test(name)) continue

    artists.push(cleanArtistName(name))
  }

  return artists
}

/**
 * Clean the event title — strip markers and series prefixes for display.
 */
function cleanTitle(title: string): { cleanedTitle: string; isSoldOut: boolean; isFree: boolean } {
  let cleanedTitle = title
  let isSoldOut = false
  let isFree = false

  if (/\*SOLD OUT\*/i.test(cleanedTitle)) {
    isSoldOut = true
    cleanedTitle = cleanedTitle.replace(/\*SOLD OUT\*\s*/i, '').trim()
  }

  if (/\*CANCELLED\*/i.test(cleanedTitle)) {
    cleanedTitle = cleanedTitle.replace(/\*CANCELLED\*\s*/i, '').trim()
  }

  if (/^FREE\b/i.test(cleanedTitle)) {
    isFree = true
    // Strip "FREE MONDAY w/ " etc. prefix but keep artist names
    cleanedTitle = cleanedTitle.replace(/^FREE\s+\w+\s+w\/\s*/i, '').trim()
  }

  return { cleanedTitle, isSoldOut, isFree }
}

// Empty Bottle provider implementation
export const emptybottleProvider: DiscoveryProvider = {
  async preview(venueSlug: string): Promise<PreviewEvent[]> {
    const venue = VENUES[venueSlug]
    if (!venue) {
      throw new Error(`Unknown venue: ${venueSlug}`)
    }

    console.log(`[emptybottle] Previewing events from ${venue.name}...`)

    const browser = await chromium.launch({ headless: true })
    const page = await browser.newPage()

    try {
      await page.goto(venue.url, { waitUntil: 'domcontentloaded', timeout: 60000 })
      await page.waitForSelector('.eb-item', { state: 'attached', timeout: 30000 })

      const rawEvents = await page.$$eval('.eb-item', (items) => {
        return items.map((item) => {
          const title = item.querySelector('.title')?.textContent?.trim() || ''
          const dateText = item.querySelector('.date')?.textContent?.trim() || ''
          const buyBtn = item.querySelector('a.buy-button') as HTMLAnchorElement | null
          const buyLink = buyBtn?.href || ''
          return { title, dateText, buyLink }
        })
      })

      const previewEvents: PreviewEvent[] = rawEvents
        .filter(e => e.buyLink)
        .map(e => {
          const { cleanedTitle } = cleanTitle(e.title)
          return {
            id: extractEventId(e.buyLink),
            title: cleanedTitle,
            date: parseDate(e.dateText),
            venue: venue.name,
          }
        })
        .filter(e => e.date !== '')

      console.log(`[emptybottle] Found ${previewEvents.length} events`)
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

    console.log(`[emptybottle] Scraping ${eventIds.length} events from ${venue.name}...`)

    const browser = await chromium.launch({ headless: true })
    const page = await browser.newPage()

    try {
      await page.goto(venue.url, { waitUntil: 'domcontentloaded', timeout: 60000 })
      await page.waitForSelector('.eb-item', { state: 'attached', timeout: 30000 })

      const eventIdSet = new Set(eventIds)

      const rawEvents = await page.$$eval('.eb-item', (items) => {
        return items.map((item) => {
          const title = item.querySelector('.title')?.textContent?.trim() || ''
          const dateText = item.querySelector('.date')?.textContent?.trim() || ''
          const startTime = item.querySelector('.start-time')?.textContent?.trim() || ''
          const artists = Array.from(item.querySelectorAll('.performing li')).map(li => li.textContent?.trim() || '')
          const restrictions = item.querySelector('.restrictions')?.textContent?.trim() || ''
          const buyBtn = item.querySelector('a.buy-button') as HTMLAnchorElement | null
          const buyLink = buyBtn?.href || ''
          const imageDiv = item.querySelector('.item-image-inner') as HTMLElement | null
          const bgStyle = imageDiv?.getAttribute('style') || ''
          const imageMatch = bgStyle.match(/url\(([^)]+)\)/)
          const imageUrl = imageMatch ? imageMatch[1].replace(/['"]/g, '') : ''

          return { title, dateText, startTime, artists, restrictions, buyLink, imageUrl }
        })
      })

      const scrapedEvents: DiscoveredEvent[] = []
      let progressCount = 0

      for (const raw of rawEvents) {
        const id = extractEventId(raw.buyLink)
        if (!eventIdSet.has(id)) continue

        progressCount++
        onProgress?.({
          current: progressCount,
          total: eventIds.length,
          eventTitle: raw.title.slice(0, 60),
          phase: 'processing',
        })

        const date = parseDate(raw.dateText)
        if (!date) continue

        const { cleanedTitle, isSoldOut, isFree } = cleanTitle(raw.title)
        const artists = cleanArtists(raw.artists)

        // Use cleaned title; fall back to first artist if title was entirely a prefix
        const displayTitle = cleanedTitle || artists[0] || raw.title

        scrapedEvents.push({
          id,
          title: displayTitle,
          date,
          venue: venue.name,
          venueSlug,
          imageUrl: raw.imageUrl || undefined,
          doorsTime: undefined, // Empty Bottle widget only shows start time
          showTime: raw.startTime ? formatTime(raw.startTime) : undefined,
          ticketUrl: raw.buyLink || undefined,
          artists: artists.length > 0 ? artists : [displayTitle],
          scrapedAt: new Date().toISOString(),
          price: isFree ? 'Free' : undefined,
          ageRestriction: raw.restrictions || undefined,
          isSoldOut: isSoldOut || undefined,
        })
      }

      console.log(`[emptybottle] Scraped ${scrapedEvents.length} events`)
      return scrapedEvents
    } finally {
      await browser.close()
    }
  },
}

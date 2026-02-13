import { chromium } from 'playwright'
import type { DiscoveredEvent, PreviewEvent, DiscoveryProvider } from './types'

// Venue configurations
const VENUES: Record<string, { name: string; url: string }> = {
  'valley-bar': {
    name: 'Valley Bar',
    url: 'https://www.valleybarphx.com/calendar/',
  },
  'crescent-ballroom': {
    name: 'Crescent Ballroom',
    url: 'https://www.crescentphx.com/calendar/',
  },
  'celebrity-theatre': {
    name: 'Celebrity Theatre',
    url: 'https://www.celebritytheatre.com/events/',
  },
}

// Helper functions
function decodeHtmlEntities(text: string | null | undefined): string {
  if (!text) return ''
  return text
    .replace(/&amp;/g, '&')
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&quot;/g, '"')
    .replace(/&#39;/g, "'")
    .replace(/&#8211;/g, '–')
    .replace(/&#8212;/g, '—')
    .replace(/&#8217;/g, "'")
    .replace(/&#8216;/g, "'")
    .replace(/&#8220;/g, '"')
    .replace(/&#8221;/g, '"')
}

function stripHtml(html: string | null | undefined): string {
  if (!html) return ''
  return html.replace(/<[^>]*>/g, '').trim()
}

function parseTime(timeStr: string | null | undefined): string | undefined {
  if (!timeStr) return undefined
  const match = timeStr.match(/(\d{1,2}:\d{2}\s*[ap]m)/i)
  return match ? match[1] : undefined
}

function extractImageUrl(imgHtml: string | null | undefined): string | undefined {
  if (!imgHtml) return undefined
  const match = imgHtml.match(/src="([^"]+)"/)
  return match ? match[1] : undefined
}

// Title case conversion
const LOWERCASE_WORDS = new Set(['a', 'an', 'the', 'and', 'but', 'or', 'for', 'nor', 'on', 'at', 'to', 'by', 'with', 'of', 'in'])
const UPPERCASE_PATTERNS = [/^(dj|mc|vs\.?|ft\.?|feat\.?)$/i, /^[A-Z]{2,4}$/]

function toTitleCase(str: string, force = false): string {
  if (!str) return str

  if (!force) {
    const upperCount = (str.match(/[A-Z]/g) || []).length
    const lowerCount = (str.match(/[a-z]/g) || []).length
    if (lowerCount > upperCount) return str
  }

  return str
    .toLowerCase()
    .split(' ')
    .map((word, index) => {
      for (const pattern of UPPERCASE_PATTERNS) {
        if (pattern.test(word)) {
          return word.toUpperCase()
        }
      }

      if (index > 0 && LOWERCASE_WORDS.has(word)) {
        return word
      }

      return word
        .split('-')
        .map(part => part.charAt(0).toUpperCase() + part.slice(1))
        .join('-')
    })
    .join(' ')
}

/**
 * Parse support act names from event description text.
 * Looks for patterns like "with special guest X", "w/ X", "featuring X", etc.
 * Returns array of extracted artist names.
 */
function parseDescriptionArtists(text: string): string[] {
  const artists: string[] = []
  if (!text) return artists

  // Patterns: "with special guest\nARTIST", "with special guests\nA, B", "w/ ARTIST", "featuring ARTIST"
  const patterns = [
    /with\s+special\s+guests?\s*\n?\s*(.+?)(?:\n\n|\n(?=[A-Z][a-z]+ \d)|$)/is,
    /(?:^|\n)w\/\s*(.+?)(?:\n\n|\n(?=[A-Z][a-z]+ \d)|$)/im,
    /featuring\s+(.+?)(?:\n\n|\n(?=[A-Z][a-z]+ \d)|$)/is,
    /(?:^|\n)support:\s*(.+?)(?:\n\n|\n(?=[A-Z][a-z]+ \d)|$)/im,
  ]

  for (const pattern of patterns) {
    const match = text.match(pattern)
    if (match) {
      // The captured group may have multiple names separated by commas, &, "and", or newlines
      const raw = match[1].trim()
      const names = raw
        .split(/\s*[,&]\s*|\s+and\s+|\s*\n\s*/)
        .map(n => n.trim())
        .filter(n => n.length > 0 && n.length < 60 && !/^\d/.test(n))
      artists.push(...names)
      break // Use the first matching pattern
    }
  }

  return artists
}

// Detail page extraction types
interface EventDetails {
  artists: string[]
  price?: string
  ageRestriction?: string
  isSoldOut: boolean
}

function extractPrice(html: string): string | undefined {
  // Check for "Free" or "No Cover"
  if (/\bfree\b/i.test(html) && !/\bfree\s*parking\b/i.test(html)) {
    return 'Free'
  }
  if (/\bno\s*cover\b/i.test(html)) {
    return 'Free'
  }
  // Extract dollar amount
  const match = html.match(/\$(\d+(?:\.\d{2})?)/)
  if (match) {
    return `$${match[1]}`
  }
  return undefined
}

function extractAgeRestriction(html: string): string | undefined {
  // Check for "All Ages"
  if (/\ball\s*ages?\b/i.test(html)) {
    return 'All Ages'
  }
  // Check for "16 and up", "21+", etc.
  const match = html.match(/(\d{1,2})\s*(?:and\s*up|\+)/i)
  if (match) {
    return `${match[1]}+`
  }
  return undefined
}

function extractSoldOutStatus(html: string): boolean {
  return /\bsold\s*out\b/i.test(html)
}

// TicketWeb provider implementation
export const ticketwebProvider: DiscoveryProvider = {
  async preview(venueSlug: string): Promise<PreviewEvent[]> {
    const venue = VENUES[venueSlug]
    if (!venue) {
      throw new Error(`Unknown venue: ${venueSlug}`)
    }

    console.log(`[ticketweb] Previewing events from ${venue.name}...`)

    const browser = await chromium.launch({ headless: true })
    const page = await browser.newPage()

    try {
      await page.goto(venue.url, {
        waitUntil: 'domcontentloaded',
        timeout: 60000,
      })

      // Wait for calendar data
      await page.waitForFunction(() => typeof (window as any).all_events !== 'undefined', {
        timeout: 30000,
        polling: 500,
      })

      // Extract events
      const events = await page.evaluate(() => {
        const allEvents = (window as any).all_events
        if (!allEvents) return []
        return allEvents.map((e: any) => ({
          id: e.id,
          title: e.title,
          date: e.start,
          venue: e.venue,
        }))
      })

      // Process events
      const previewEvents: PreviewEvent[] = events.map((e: any) => ({
        id: e.id,
        title: toTitleCase(decodeHtmlEntities(e.title)),
        date: e.date,
        venue: stripHtml(e.venue) || venue.name,
      }))

      console.log(`[ticketweb] Found ${previewEvents.length} events`)
      return previewEvents
    } finally {
      await browser.close()
    }
  },

  async scrape(venueSlug: string, eventIds: string[]): Promise<DiscoveredEvent[]> {
    const venue = VENUES[venueSlug]
    if (!venue) {
      throw new Error(`Unknown venue: ${venueSlug}`)
    }

    console.log(`[ticketweb] Scraping ${eventIds.length} events from ${venue.name}...`)

    const browser = await chromium.launch({ headless: true })
    const page = await browser.newPage()

    try {
      await page.goto(venue.url, {
        waitUntil: 'domcontentloaded',
        timeout: 60000,
      })

      // Wait for calendar data
      await page.waitForFunction(() => typeof (window as any).all_events !== 'undefined', {
        timeout: 30000,
        polling: 500,
      })

      // Get all events and filter to selected ones
      let events = await page.evaluate(() => {
        return (window as any).all_events || []
      })

      const eventIdSet = new Set(eventIds)
      events = events.filter((e: any) => eventIdSet.has(e.id))

      // Get ticket links
      const ticketLinks = await page.evaluate(() => {
        const links: Record<string, string> = {}
        document.querySelectorAll('[id^="tw-event-dialog-"] a[href*="ticketweb"]').forEach((a) => {
          const dialog = (a as HTMLElement).closest('[id^="tw-event-dialog-"]')
          if (dialog) {
            const id = dialog.id.replace('tw-event-dialog-', '')
            links[id] = (a as HTMLAnchorElement).href
          }
        })
        return links
      })

      // Get event detail URLs
      const eventUrls = await page.evaluate(() => {
        const urls: Record<string, string> = {}
        document.querySelectorAll('[id^="tw-event-dialog-"] .tw-name a').forEach((a) => {
          const dialog = (a as HTMLElement).closest('[id^="tw-event-dialog-"]')
          if (dialog) {
            const id = dialog.id.replace('tw-event-dialog-', '')
            urls[id] = (a as HTMLAnchorElement).href
          }
        })
        return urls
      })

      // Fetch details from detail pages using Playwright (HTTP gets blocked by bot detection)
      const detailsByEventId: Record<string, EventDetails> = {}

      for (const event of events) {
        const detailUrl = eventUrls[event.id]
        if (!detailUrl) continue

        try {
          const detailPage = await browser.newPage()
          await detailPage.goto(detailUrl, { waitUntil: 'networkidle', timeout: 15000 })

          const details = await detailPage.evaluate(() => {
            // Extract artists from .artist-list h4 a elements
            const artists: string[] = []
            document.querySelectorAll('.artist-list h4 a').forEach((el) => {
              const name = el.textContent?.trim()
              if (name) artists.push(name)
            })

            // Extract description text for support act parsing
            const descEl = document.querySelector('.tw-event-description, .event-description, [class*="description"]')
            const descText = (descEl as HTMLElement)?.innerText?.trim() || ''

            // Also check full body text for support acts listed outside artist-list
            const bodyText = document.body.innerText || ''

            return { artists, descText, bodyText }
          })

          // Parse support acts from description text
          const supportArtists = parseDescriptionArtists(details.descText || details.bodyText)

          // Merge: artist-list names first, then description-parsed support acts (deduped)
          const allArtists = [...details.artists]
          const lowerArtists = new Set(allArtists.map(a => a.toLowerCase()))
          for (const sa of supportArtists) {
            if (!lowerArtists.has(sa.toLowerCase())) {
              allArtists.push(sa)
              lowerArtists.add(sa.toLowerCase())
            }
          }

          const html = await detailPage.content()
          detailsByEventId[event.id] = {
            artists: allArtists.map(name => toTitleCase(name, true)),
            price: extractPrice(html),
            ageRestriction: extractAgeRestriction(html),
            isSoldOut: extractSoldOutStatus(html),
          }

          await detailPage.close()
        } catch (err) {
          console.warn(`[ticketweb] Failed to fetch detail page for event ${event.id}:`, err instanceof Error ? err.message : err)
        }
      }

      // Process each event
      const scrapedEvents: DiscoveredEvent[] = []

      for (let i = 0; i < events.length; i++) {
        const event = events[i]

        console.log(`[ticketweb] [${i + 1}/${events.length}] Processing: ${decodeHtmlEntities(event.title).slice(0, 40)}...`)

        const details = detailsByEventId[event.id]
        let artists = details?.artists || []

        // Fall back to title if no artists found
        if (artists.length === 0) {
          const title = toTitleCase(decodeHtmlEntities(event.title))
          const cleanTitle = title.replace(/\s*[–-]\s*[^–-]*tour.*$/i, '').trim()
          artists = [cleanTitle]
        }

        scrapedEvents.push({
          id: event.id,
          title: toTitleCase(decodeHtmlEntities(event.title)),
          date: event.start,
          venue: stripHtml(event.venue) || venue.name,
          venueSlug: venueSlug,
          imageUrl: extractImageUrl(event.imageUrl),
          doorsTime: parseTime(event.doors),
          showTime: parseTime(event.displayTime),
          ticketUrl: ticketLinks[event.id] || undefined,
          artists: artists,
          scrapedAt: new Date().toISOString(),
          price: details?.price,
          ageRestriction: details?.ageRestriction,
          isSoldOut: details?.isSoldOut || undefined,
        })
      }

      console.log(`[ticketweb] Scraped ${scrapedEvents.length} events`)
      return scrapedEvents
    } finally {
      await browser.close()
    }
  },
}

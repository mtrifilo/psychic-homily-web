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
  'the-van-buren': {
    name: 'The Van Buren',
    url: 'https://thevanburenphx.com/calendar/',
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

      // Process each event
      const scrapedEvents: DiscoveredEvent[] = []

      for (let i = 0; i < events.length; i++) {
        const event = events[i]
        const eventUrl = eventUrls[event.id]

        console.log(`[ticketweb] [${i + 1}/${events.length}] Scraping: ${decodeHtmlEntities(event.title).slice(0, 40)}...`)

        let artists: string[] = []

        // Fetch artist list from detail page
        if (eventUrl) {
          let detailPage: Awaited<ReturnType<typeof browser.newPage>> | null = null
          try {
            detailPage = await browser.newPage()

            // Race the page load against an absolute timeout
            const timeoutMs = 20000
            const result = await Promise.race([
              (async () => {
                await detailPage!.goto(eventUrl, { waitUntil: 'domcontentloaded', timeout: 15000 })
                return await detailPage!.evaluate(() => {
                  const artistList: string[] = []
                  document.querySelectorAll('.artist-list .row h4 a').forEach((a) => {
                    const name = a.textContent?.trim()
                    if (name) artistList.push(name)
                  })
                  return artistList
                })
              })(),
              new Promise<null>((_, reject) =>
                setTimeout(() => reject(new Error('Detail page timeout')), timeoutMs)
              ),
            ])

            if (result) {
              artists = result.map(name => toTitleCase(name, true))
            }
          } catch (err) {
            console.warn(`[ticketweb] Failed to fetch detail page for event ${event.id}:`, err instanceof Error ? err.message : err)
            artists = []
          } finally {
            try { await detailPage?.close() } catch { /* ignore close errors */ }
          }
        }

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
        })
      }

      console.log(`[ticketweb] Scraped ${scrapedEvents.length} events`)
      return scrapedEvents
    } finally {
      await browser.close()
    }
  },
}

import { getScraper } from './scrapers'

const PORT = 3001

// Maximum concurrent scraping operations
const MAX_CONCURRENT_SCRAPES = 5

// Venue configurations (should match the scrapers)
const VENUES: Record<string, { name: string; scraperType: string; city: string; state: string }> = {
  // Phoenix, AZ - Stateside Presents venues (TicketWeb)
  'valley-bar': { name: 'Valley Bar', scraperType: 'ticketweb', city: 'Phoenix', state: 'AZ' },
  'crescent-ballroom': { name: 'Crescent Ballroom', scraperType: 'ticketweb', city: 'Phoenix', state: 'AZ' },
  'the-van-buren': { name: 'The Van Buren', scraperType: 'ticketweb', city: 'Phoenix', state: 'AZ' },
  'celebrity-theatre': { name: 'Celebrity Theatre', scraperType: 'ticketweb', city: 'Phoenix', state: 'AZ' },
  'arizona-financial-theatre': { name: 'Arizona Financial Theatre', scraperType: 'ticketweb', city: 'Phoenix', state: 'AZ' },

  // Denver, CO - Sample venues (would need proper scraper implementation)
  // 'gothic-theatre': { name: 'Gothic Theatre', scraperType: 'ticketweb', city: 'Denver', state: 'CO' },
  // 'bluebird-theater': { name: 'Bluebird Theater', scraperType: 'ticketweb', city: 'Denver', state: 'CO' },

  // Austin, TX - Sample venues
  // 'mohawk': { name: 'Mohawk', scraperType: 'other', city: 'Austin', state: 'TX' },
}

// CORS headers
const corsHeaders = {
  'Access-Control-Allow-Origin': '*',
  'Access-Control-Allow-Methods': 'GET, POST, OPTIONS',
  'Access-Control-Allow-Headers': 'Content-Type',
}

// JSON response helper
function jsonResponse(data: unknown, status = 200) {
  return new Response(JSON.stringify(data), {
    status,
    headers: {
      'Content-Type': 'application/json',
      ...corsHeaders,
    },
  })
}

// Error response helper
function errorResponse(message: string, status = 400) {
  return jsonResponse({ error: message }, status)
}

// Main server
const server = Bun.serve({
  port: PORT,
  async fetch(req) {
    const url = new URL(req.url)
    const path = url.pathname

    // Handle CORS preflight
    if (req.method === 'OPTIONS') {
      return new Response(null, { status: 204, headers: corsHeaders })
    }

    // Routes
    try {
      // GET /scraper/venues - List available venues
      if (path === '/scraper/venues' && req.method === 'GET') {
        const venueList = Object.entries(VENUES).map(([slug, config]) => ({
          slug,
          name: config.name,
          scraperType: config.scraperType,
          city: config.city,
          state: config.state,
        }))
        return jsonResponse(venueList)
      }

      // POST /scraper/preview-batch - Preview multiple venues in parallel
      if (path === '/scraper/preview-batch' && req.method === 'POST') {
        const body = await req.json() as { venueSlugs?: string[] }
        const venueSlugs = body.venueSlugs || []

        if (!Array.isArray(venueSlugs) || venueSlugs.length === 0) {
          return errorResponse('venueSlugs array is required')
        }

        console.log(`[server] Batch preview request for ${venueSlugs.length} venues`)

        // Process venues in parallel with concurrency limit
        const results: Array<{ venueSlug: string; events?: Array<{ id: string; title: string; date: string; venue: string }>; error?: string }> = []

        // Process in batches of MAX_CONCURRENT_SCRAPES
        for (let i = 0; i < venueSlugs.length; i += MAX_CONCURRENT_SCRAPES) {
          const batch = venueSlugs.slice(i, i + MAX_CONCURRENT_SCRAPES)
          const batchPromises = batch.map(async (slug) => {
            const venueConfig = VENUES[slug]
            if (!venueConfig) {
              return { venueSlug: slug, error: `Unknown venue: ${slug}` }
            }

            const scraper = getScraper(venueConfig.scraperType)
            if (!scraper) {
              return { venueSlug: slug, error: `No scraper for type: ${venueConfig.scraperType}` }
            }

            try {
              console.log(`[server] Previewing ${slug}...`)
              const events = await scraper.preview(slug)
              console.log(`[server] ${slug}: ${events.length} events found`)
              return { venueSlug: slug, events }
            } catch (err) {
              const message = err instanceof Error ? err.message : 'Unknown error'
              console.error(`[server] Error previewing ${slug}:`, message)
              return { venueSlug: slug, error: message }
            }
          })

          const batchResults = await Promise.all(batchPromises)
          results.push(...batchResults)
        }

        return jsonResponse(results)
      }

      // GET /scraper/preview/:venueSlug - Quick preview of events (single venue)
      if (path.startsWith('/scraper/preview/') && req.method === 'GET') {
        const venueSlug = path.replace('/scraper/preview/', '')
        const venueConfig = VENUES[venueSlug]

        if (!venueConfig) {
          return errorResponse(`Unknown venue: ${venueSlug}`, 404)
        }

        const scraper = getScraper(venueConfig.scraperType)
        if (!scraper) {
          return errorResponse(`No scraper for type: ${venueConfig.scraperType}`, 500)
        }

        console.log(`[server] Preview request for ${venueSlug}`)
        const events = await scraper.preview(venueSlug)
        return jsonResponse(events)
      }

      // POST /scraper/scrape/:venueSlug - Scrape selected events
      if (path.startsWith('/scraper/scrape/') && req.method === 'POST') {
        const venueSlug = path.replace('/scraper/scrape/', '')
        const venueConfig = VENUES[venueSlug]

        if (!venueConfig) {
          return errorResponse(`Unknown venue: ${venueSlug}`, 404)
        }

        const scraper = getScraper(venueConfig.scraperType)
        if (!scraper) {
          return errorResponse(`No scraper for type: ${venueConfig.scraperType}`, 500)
        }

        const body = await req.json() as { eventIds?: string[] }
        const eventIds = body.eventIds || []

        if (!Array.isArray(eventIds) || eventIds.length === 0) {
          return errorResponse('eventIds array is required')
        }

        console.log(`[server] Scrape request for ${venueSlug}, ${eventIds.length} events`)
        const events = await scraper.scrape(venueSlug, eventIds)
        return jsonResponse(events)
      }

      // Health check
      if (path === '/scraper/health') {
        return jsonResponse({ status: 'ok', timestamp: new Date().toISOString() })
      }

      // 404 for unknown routes
      return errorResponse('Not found', 404)
    } catch (error) {
      console.error('[server] Error:', error)
      const message = error instanceof Error ? error.message : 'Internal server error'
      return errorResponse(message, 500)
    }
  },
})

console.log(`
╔═══════════════════════════════════════════════════════════╗
║           Psychic Homily Scraper Server                   ║
║                                                           ║
║  Running on http://localhost:${PORT}                        ║
║                                                           ║
║  Endpoints:                                               ║
║    GET  /scraper/venues          - List venues            ║
║    GET  /scraper/preview/:slug   - Preview events         ║
║    POST /scraper/preview-batch   - Parallel preview       ║
║    POST /scraper/scrape/:slug    - Scrape events          ║
║    GET  /scraper/health          - Health check           ║
║                                                           ║
║  Configured venues: ${Object.keys(VENUES).length}                                  ║
╚═══════════════════════════════════════════════════════════╝
`)

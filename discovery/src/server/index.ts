import { getProvider } from './providers'

const PORT = 3001

// Maximum concurrent discovery operations
const MAX_CONCURRENT_SCRAPES = 5

// Venue configurations (should match the providers)
const VENUES: Record<string, { name: string; providerType: string; city: string; state: string }> = {
  // Phoenix, AZ - Stateside Presents venues (TicketWeb)
  'valley-bar': { name: 'Valley Bar', providerType: 'ticketweb', city: 'Phoenix', state: 'AZ' },
  'crescent-ballroom': { name: 'Crescent Ballroom', providerType: 'ticketweb', city: 'Phoenix', state: 'AZ' },
  'the-van-buren': { name: 'The Van Buren', providerType: 'jsonld', city: 'Phoenix', state: 'AZ' },
  'celebrity-theatre': { name: 'Celebrity Theatre', providerType: 'ticketweb', city: 'Phoenix', state: 'AZ' },
  'arizona-financial-theatre': { name: 'Arizona Financial Theatre', providerType: 'jsonld', city: 'Phoenix', state: 'AZ' },

  // Denver, CO - Sample venues (would need proper provider implementation)
  // 'gothic-theatre': { name: 'Gothic Theatre', providerType: 'ticketweb', city: 'Denver', state: 'CO' },
  // 'bluebird-theater': { name: 'Bluebird Theater', providerType: 'ticketweb', city: 'Denver', state: 'CO' },

  // Austin, TX - Sample venues
  // 'mohawk': { name: 'Mohawk', providerType: 'other', city: 'Austin', state: 'TX' },
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
      // GET /discovery/venues - List available venues
      if (path === '/discovery/venues' && req.method === 'GET') {
        const venueList = Object.entries(VENUES).map(([slug, config]) => ({
          slug,
          name: config.name,
          providerType: config.providerType,
          city: config.city,
          state: config.state,
        }))
        return jsonResponse(venueList)
      }

      // POST /discovery/preview-batch - Preview multiple venues in parallel
      if (path === '/discovery/preview-batch' && req.method === 'POST') {
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

            const provider = getProvider(venueConfig.providerType)
            if (!provider) {
              return { venueSlug: slug, error: `No provider for type: ${venueConfig.providerType}` }
            }

            try {
              console.log(`[server] Previewing ${slug}...`)
              const events = await provider.preview(slug)
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

      // GET /discovery/preview/:venueSlug - Quick preview of events (single venue)
      if (path.startsWith('/discovery/preview/') && req.method === 'GET') {
        const venueSlug = path.replace('/discovery/preview/', '')
        const venueConfig = VENUES[venueSlug]

        if (!venueConfig) {
          return errorResponse(`Unknown venue: ${venueSlug}`, 404)
        }

        const provider = getProvider(venueConfig.providerType)
        if (!provider) {
          return errorResponse(`No provider for type: ${venueConfig.providerType}`, 500)
        }

        console.log(`[server] Preview request for ${venueSlug}`)
        const events = await provider.preview(venueSlug)
        return jsonResponse(events)
      }

      // POST /discovery/scrape/:venueSlug - Scrape selected events
      if (path.startsWith('/discovery/scrape/') && req.method === 'POST') {
        const venueSlug = path.replace('/discovery/scrape/', '')
        const venueConfig = VENUES[venueSlug]

        if (!venueConfig) {
          return errorResponse(`Unknown venue: ${venueSlug}`, 404)
        }

        const provider = getProvider(venueConfig.providerType)
        if (!provider) {
          return errorResponse(`No provider for type: ${venueConfig.providerType}`, 500)
        }

        const body = await req.json() as { eventIds?: string[] }
        const eventIds = body.eventIds || []

        if (!Array.isArray(eventIds) || eventIds.length === 0) {
          return errorResponse('eventIds array is required')
        }

        console.log(`[server] Scrape request for ${venueSlug}, ${eventIds.length} events`)
        const events = await provider.scrape(venueSlug, eventIds)
        return jsonResponse(events)
      }

      // Health check
      if (path === '/discovery/health') {
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

const bannerLines = [
  `           Psychic Homily Discovery Server`,
  ``,
  `  Running on http://localhost:${PORT}`,
  ``,
  `  Endpoints:`,
  `    GET  /discovery/venues          - List venues`,
  `    GET  /discovery/preview/:slug   - Preview events`,
  `    POST /discovery/preview-batch   - Parallel preview`,
  `    POST /discovery/scrape/:slug    - Scrape events`,
  `    GET  /discovery/health          - Health check`,
  ``,
  `  Configured venues: ${Object.keys(VENUES).length}`,
]
const width = 59
console.log(`\n╔${'═'.repeat(width)}╗`)
for (const line of bannerLines) {
  console.log(`║${line.padEnd(width)}║`)
}
console.log(`╚${'═'.repeat(width)}╝\n`)

import type { VenueConfig } from './types'

// Configured venues that can be scraped
// NOTE: Venues must also be configured in:
//   - scraper/src/server/index.ts (server config)
//   - backend/internal/services/scraper.go (Go backend VenueConfig)
export const VENUES: VenueConfig[] = [
  // Phoenix, AZ - Stateside Presents venues (TicketWeb)
  {
    slug: 'valley-bar',
    name: 'Valley Bar',
    scraperType: 'ticketweb',
    url: 'https://www.valleybarphx.com/calendar/',
    city: 'Phoenix',
    state: 'AZ',
  },
  {
    slug: 'crescent-ballroom',
    name: 'Crescent Ballroom',
    scraperType: 'ticketweb',
    url: 'https://www.crescentphx.com/calendar/',
    city: 'Phoenix',
    state: 'AZ',
  },
  {
    slug: 'the-van-buren',
    name: 'The Van Buren',
    scraperType: 'ticketweb',
    url: 'https://thevanburenphx.com/calendar/',
    city: 'Phoenix',
    state: 'AZ',
  },
  {
    slug: 'celebrity-theatre',
    name: 'Celebrity Theatre',
    scraperType: 'ticketweb',
    url: 'https://www.celebritytheatre.com/events/',
    city: 'Phoenix',
    state: 'AZ',
  },
  {
    slug: 'arizona-financial-theatre',
    name: 'Arizona Financial Theatre',
    scraperType: 'ticketweb',
    url: 'https://www.livenation.com/venue/KovZpZAEkn1A/arizona-financial-theatre-events',
    city: 'Phoenix',
    state: 'AZ',
  },

  // NOTE: Add more venues here as you implement scrapers for them.
  // Example venues from other cities (would need proper scraper implementation):
  //
  // Denver, CO
  // { slug: 'gothic-theatre', name: 'Gothic Theatre', scraperType: 'other', url: '...', city: 'Denver', state: 'CO' },
  //
  // Austin, TX
  // { slug: 'mohawk', name: 'Mohawk', scraperType: 'other', url: '...', city: 'Austin', state: 'TX' },
]

// Get unique cities with venue counts
export function getVenueCities(): { city: string; state: string; count: number }[] {
  const cityMap = new Map<string, { city: string; state: string; count: number }>()
  for (const venue of VENUES) {
    const key = `${venue.city}, ${venue.state}`
    const existing = cityMap.get(key)
    if (existing) {
      existing.count++
    } else {
      cityMap.set(key, { city: venue.city, state: venue.state, count: 1 })
    }
  }
  return Array.from(cityMap.values()).sort((a, b) => a.city.localeCompare(b.city))
}

// Scraper server URL (local Bun server)
export const SCRAPER_API_URL = '/scraper'

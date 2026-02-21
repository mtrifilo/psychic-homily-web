import type { VenueConfig } from './types'

// Configured venues for discovery
// NOTE: Venues must also be configured in:
//   - discovery/src/server/index.ts (server config)
//   - backend/internal/services/discovery.go (Go backend VenueConfig)
export const VENUES: VenueConfig[] = [
  // Phoenix, AZ - Stateside Presents venues (TicketWeb)
  {
    slug: 'valley-bar',
    name: 'Valley Bar',
    providerType: 'ticketweb',
    url: 'https://www.valleybarphx.com/calendar/',
    city: 'Phoenix',
    state: 'AZ',
  },
  {
    slug: 'crescent-ballroom',
    name: 'Crescent Ballroom',
    providerType: 'ticketweb',
    url: 'https://www.crescentphx.com/calendar/',
    city: 'Phoenix',
    state: 'AZ',
  },
  {
    slug: 'the-van-buren',
    name: 'The Van Buren',
    providerType: 'jsonld',
    url: 'https://thevanburenphx.com/shows',
    city: 'Phoenix',
    state: 'AZ',
  },
  {
    slug: 'celebrity-theatre',
    name: 'Celebrity Theatre',
    providerType: 'wix',
    url: 'https://www.celebritytheatre.com/events/',
    city: 'Phoenix',
    state: 'AZ',
  },
  {
    slug: 'arizona-financial-theatre',
    name: 'Arizona Financial Theatre',
    providerType: 'jsonld',
    url: 'https://www.arizonafinancialtheatre.com/shows',
    city: 'Phoenix',
    state: 'AZ',
  },

  // Chicago, IL
  {
    slug: 'empty-bottle',
    name: 'Empty Bottle',
    providerType: 'emptybottle',
    url: 'https://www.emptybottle.com/',
    city: 'Chicago',
    state: 'IL',
  },

  // Phoenix, AZ - SeeTickets venues
  {
    slug: 'the-rebel-lounge',
    name: 'The Rebel Lounge',
    providerType: 'seetickets',
    url: 'https://therebellounge.com/events/',
    city: 'Phoenix',
    state: 'AZ',
  },

  // NOTE: Add more venues here as you implement providers for them.
  // Example venues from other cities (would need proper provider implementation):
  //
  // Denver, CO
  // { slug: 'gothic-theatre', name: 'Gothic Theatre', providerType: 'other', url: '...', city: 'Denver', state: 'CO' },
  //
  // Austin, TX
  // { slug: 'mohawk', name: 'Mohawk', providerType: 'other', url: '...', city: 'Austin', state: 'TX' },
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

// Discovery server URL (local Bun server)
export const DISCOVERY_API_URL = '/discovery'

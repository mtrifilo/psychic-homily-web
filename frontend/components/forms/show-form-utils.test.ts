import { describe, it, expect } from 'vitest'
import {
  showToFormValues,
  parseCost,
  removeArtistAtIndex,
  isVenueLocationEditable,
  defaultFormValues,
  type FormArtist,
} from './show-form-utils'
import type { ShowResponse, VenueResponse } from '@/lib/types/show'

// --- Helpers ---

function makeShowResponse(overrides?: Partial<ShowResponse>): ShowResponse {
  return {
    id: 1,
    slug: 'test-show',
    title: 'Test Show',
    event_date: '2026-03-15T03:00:00Z', // 8pm MST (America/Phoenix = UTC-7)
    city: 'Phoenix',
    state: 'AZ',
    price: 20,
    age_requirement: '21+',
    description: 'A great show',
    status: 'approved',
    venues: [
      {
        id: 10,
        slug: 'the-venue',
        name: 'The Venue',
        address: '123 Main St',
        city: 'Phoenix',
        state: 'AZ',
        verified: true,
      },
    ],
    artists: [
      {
        id: 100,
        slug: 'artist-one',
        name: 'Artist One',
        is_headliner: true,
        socials: {},
      },
      {
        id: 101,
        slug: 'artist-two',
        name: 'Artist Two',
        is_headliner: false,
        socials: {},
      },
    ],
    created_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeVenueResponse(overrides?: Partial<VenueResponse>): VenueResponse {
  return {
    id: 1,
    slug: 'test-venue',
    name: 'Test Venue',
    city: 'Phoenix',
    state: 'AZ',
    verified: true,
    ...overrides,
  }
}

// --- showToFormValues ---

describe('showToFormValues', () => {
  it('maps basic show fields', () => {
    const show = makeShowResponse()
    const result = showToFormValues(show)

    expect(result.title).toBe('Test Show')
    expect(result.cost).toBe('$20')
    expect(result.ages).toBe('21+')
    expect(result.description).toBe('A great show')
  })

  it('maps venue from first venue in array', () => {
    const show = makeShowResponse()
    const result = showToFormValues(show)

    expect(result.venue.name).toBe('The Venue')
    expect(result.venue.city).toBe('Phoenix')
    expect(result.venue.state).toBe('AZ')
    expect(result.venue.address).toBe('123 Main St')
  })

  it('maps artists with headliner status', () => {
    const show = makeShowResponse()
    const result = showToFormValues(show)

    expect(result.artists).toHaveLength(2)
    expect(result.artists[0].name).toBe('Artist One')
    expect(result.artists[0].is_headliner).toBe(true)
    expect(result.artists[0].matched_id).toBe(100)
    expect(result.artists[1].name).toBe('Artist Two')
    expect(result.artists[1].is_headliner).toBe(false)
  })

  it('parses date and time in venue timezone', () => {
    // 2026-03-15T03:00:00Z = 2026-03-14 at 20:00 MST (UTC-7)
    const show = makeShowResponse({ event_date: '2026-03-15T03:00:00Z' })
    const result = showToFormValues(show)

    expect(result.date).toBe('2026-03-14')
    expect(result.time).toBe('20:00')
  })

  it('returns empty cost when price is null', () => {
    const show = makeShowResponse({ price: null })
    expect(showToFormValues(show).cost).toBe('')
  })

  it('returns empty cost when price is undefined', () => {
    const show = makeShowResponse({ price: undefined })
    expect(showToFormValues(show).cost).toBe('')
  })

  it('returns "$0" when price is 0', () => {
    const show = makeShowResponse({ price: 0 })
    expect(showToFormValues(show).cost).toBe('$0')
  })

  it('falls back to show city/state when venue has none', () => {
    const show = makeShowResponse({
      city: 'Tucson',
      state: 'AZ',
      venues: [{ id: 1, slug: 's', name: 'V', city: '', state: '', verified: false }],
    })
    const result = showToFormValues(show)

    expect(result.venue.city).toBe('Tucson')
    expect(result.venue.state).toBe('AZ')
  })

  it('handles empty venues array gracefully', () => {
    const show = makeShowResponse({ venues: [] as VenueResponse[], city: 'Mesa', state: 'AZ' })
    const result = showToFormValues(show)

    expect(result.venue.name).toBe('')
    expect(result.venue.city).toBe('Mesa')
    expect(result.venue.state).toBe('AZ')
  })

  it('handles null description, age_requirement, title', () => {
    const show = makeShowResponse({
      title: '',
      description: null,
      age_requirement: null,
    })
    const result = showToFormValues(show)

    expect(result.title).toBe('')
    expect(result.description).toBe('')
    expect(result.ages).toBe('')
  })
})

// --- parseCost ---

describe('parseCost', () => {
  it('parses "$20" to 20', () => {
    expect(parseCost('$20')).toBe(20)
  })

  it('parses "$12.50" to 12.5', () => {
    expect(parseCost('$12.50')).toBe(12.5)
  })

  it('parses "15" to 15', () => {
    expect(parseCost('15')).toBe(15)
  })

  it('returns undefined for "Free"', () => {
    expect(parseCost('Free')).toBeUndefined()
  })

  it('returns undefined for empty string', () => {
    expect(parseCost('')).toBeUndefined()
  })

  it('parses "$0" to 0', () => {
    expect(parseCost('$0')).toBe(0)
  })

  it('parses "$5 suggested donation" to 5', () => {
    expect(parseCost('$5 suggested donation')).toBe(5)
  })
})

// --- removeArtistAtIndex ---

describe('removeArtistAtIndex', () => {
  const headliner: FormArtist = { name: 'Head', is_headliner: true, matched_id: 1 }
  const opener: FormArtist = { name: 'Opener', is_headliner: false, matched_id: 2 }
  const support: FormArtist = { name: 'Support', is_headliner: false, matched_id: 3 }

  it('returns null when only one artist remains', () => {
    expect(removeArtistAtIndex([headliner], 0)).toBeNull()
  })

  it('removes the artist at the given index', () => {
    const result = removeArtistAtIndex([headliner, opener, support], 1)!
    expect(result).toHaveLength(2)
    expect(result.map(a => a.name)).toEqual(['Head', 'Support'])
  })

  it('promotes first remaining artist to headliner when headliner is removed', () => {
    const result = removeArtistAtIndex([headliner, opener, support], 0)!
    expect(result[0].is_headliner).toBe(true)
    expect(result[0].name).toBe('Opener')
  })

  it('does not change headliner status when non-headliner is removed', () => {
    const result = removeArtistAtIndex([headliner, opener], 1)!
    expect(result[0].is_headliner).toBe(true)
    expect(result[0].name).toBe('Head')
  })

  it('does not mutate the original array', () => {
    const artists = [headliner, opener]
    removeArtistAtIndex(artists, 1)
    expect(artists).toHaveLength(2)
  })
})

// --- isVenueLocationEditable ---

describe('isVenueLocationEditable', () => {
  const verifiedVenue = makeVenueResponse({ verified: true })
  const unverifiedVenue = makeVenueResponse({ verified: false })

  it('returns false when prefilled venue exists (regardless of other factors)', () => {
    expect(isVenueLocationEditable(true, null, true)).toBe(false)
    expect(isVenueLocationEditable(false, null, true)).toBe(false)
  })

  it('returns true for admin (even with verified venue)', () => {
    expect(isVenueLocationEditable(true, verifiedVenue, false)).toBe(true)
  })

  it('returns true when no venue is selected', () => {
    expect(isVenueLocationEditable(false, null, false)).toBe(true)
  })

  it('returns true for unverified venue (non-admin)', () => {
    expect(isVenueLocationEditable(false, unverifiedVenue, false)).toBe(true)
  })

  it('returns false for verified venue (non-admin)', () => {
    expect(isVenueLocationEditable(false, verifiedVenue, false)).toBe(false)
  })
})

// --- defaultFormValues ---

describe('defaultFormValues', () => {
  it('has one artist with headliner status', () => {
    expect(defaultFormValues.artists).toHaveLength(1)
    expect(defaultFormValues.artists[0].is_headliner).toBe(true)
    expect(defaultFormValues.artists[0].name).toBe('')
  })

  it('has default time of 20:00', () => {
    expect(defaultFormValues.time).toBe('20:00')
  })
})

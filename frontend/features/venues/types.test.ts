import { describe, it, expect } from 'vitest'
import { getVenueLocation, type Venue } from './types'

function venue(overrides: Partial<Venue> = {}): Venue {
  return {
    id: 1,
    slug: 'the-club',
    name: 'The Club',
    address: null,
    city: 'Phoenix',
    state: 'AZ',
    verified: true,
    created_at: '2026-05-19T12:00:00Z',
    updated_at: '2026-05-19T12:00:00Z',
    ...overrides,
  }
}

describe('getVenueLocation', () => {
  it('joins city and state with a comma', () => {
    expect(getVenueLocation(venue())).toBe('Phoenix, AZ')
  })

  it('reflects the venue city and state passed in', () => {
    expect(getVenueLocation(venue({ city: 'Austin', state: 'TX' }))).toBe(
      'Austin, TX'
    )
  })

  it('still renders the separator when a part is an empty string', () => {
    // The helper is a plain template join with no null/empty guard; an empty
    // state leaves a trailing ", " so callers can spot missing data.
    expect(getVenueLocation(venue({ state: '' }))).toBe('Phoenix, ')
  })
})

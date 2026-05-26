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

  it('drops the separator when state is empty (PSY-780 fix)', () => {
    // Previously the helper rendered "Phoenix, " (trailing comma + space) for
    // venues with a missing state. Now delegates to the shared formatLocation
    // helper, which filters empty parts before joining.
    expect(getVenueLocation(venue({ state: '' }))).toBe('Phoenix')
  })

  it('renders only the state when city is empty', () => {
    expect(getVenueLocation(venue({ city: '', state: 'AZ' }))).toBe('AZ')
  })

  it('returns "Location Unknown" when both city and state are empty', () => {
    expect(getVenueLocation(venue({ city: '', state: '' }))).toBe(
      'Location Unknown'
    )
  })
})

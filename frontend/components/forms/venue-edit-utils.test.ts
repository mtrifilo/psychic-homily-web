import { describe, it, expect } from 'vitest'
import { detectVenueChanges, type VenueEditFormValues } from './venue-edit-utils'
import type { Venue, VenueWithShowCount } from '@/lib/types/venue'

// --- Helpers ---

function makeVenue(overrides?: Partial<VenueWithShowCount>): VenueWithShowCount {
  return {
    id: 1,
    slug: 'test-venue',
    name: 'Test Venue',
    address: '123 Main St',
    city: 'Phoenix',
    state: 'AZ',
    zipcode: '85001',
    verified: true,
    upcoming_show_count: 5,
    social: {
      instagram: 'https://instagram.com/testvenue',
      facebook: '',
      twitter: null,
      youtube: null,
      spotify: null,
      soundcloud: null,
      bandcamp: null,
      website: 'https://testvenue.com',
    },
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeFormValues(overrides?: Partial<VenueEditFormValues>): VenueEditFormValues {
  return {
    name: 'Test Venue',
    address: '123 Main St',
    city: 'Phoenix',
    state: 'AZ',
    zipcode: '85001',
    instagram: 'https://instagram.com/testvenue',
    facebook: '',
    twitter: '',
    youtube: '',
    spotify: '',
    soundcloud: '',
    bandcamp: '',
    website: 'https://testvenue.com',
    ...overrides,
  }
}

describe('detectVenueChanges', () => {
  it('returns null when no changes detected', () => {
    const venue = makeVenue()
    const values = makeFormValues()

    expect(detectVenueChanges(values, venue)).toBeNull()
  })

  it('detects name change', () => {
    const venue = makeVenue()
    const values = makeFormValues({ name: 'New Venue Name' })

    const result = detectVenueChanges(values, venue)
    expect(result).toEqual({ name: 'New Venue Name' })
  })

  it('detects city change only', () => {
    const venue = makeVenue()
    const values = makeFormValues({ city: 'Tucson' })

    const result = detectVenueChanges(values, venue)
    expect(result).toEqual({ city: 'Tucson' })
  })

  it('detects multiple field changes', () => {
    const venue = makeVenue()
    const values = makeFormValues({
      name: 'Updated Venue',
      city: 'Tempe',
      state: 'CA',
    })

    const result = detectVenueChanges(values, venue)
    expect(result).toEqual({
      name: 'Updated Venue',
      city: 'Tempe',
      state: 'CA',
    })
  })

  it('detects address change', () => {
    const venue = makeVenue()
    const values = makeFormValues({ address: '456 Oak Ave' })

    const result = detectVenueChanges(values, venue)
    expect(result).toEqual({ address: '456 Oak Ave' })
  })

  it('sets address to undefined when clearing (empty string)', () => {
    const venue = makeVenue({ address: '123 Main St' })
    const values = makeFormValues({ address: '' })

    const result = detectVenueChanges(values, venue)
    expect(result).toEqual({ address: undefined })
  })

  it('handles venue with null address (no change when form is empty)', () => {
    const venue = makeVenue({ address: null })
    const values = makeFormValues({ address: '' })

    // address: '' === (venue.address || '') === '' — no change
    expect(detectVenueChanges(values, venue)).toBeNull()
  })

  it('detects zipcode change', () => {
    const venue = makeVenue()
    const values = makeFormValues({ zipcode: '85004' })

    const result = detectVenueChanges(values, venue)
    expect(result).toEqual({ zipcode: '85004' })
  })

  it('sets zipcode to undefined when clearing', () => {
    const venue = makeVenue({ zipcode: '85001' })
    const values = makeFormValues({ zipcode: '' })

    const result = detectVenueChanges(values, venue)
    expect(result).toEqual({ zipcode: undefined })
  })

  it('detects social link changes', () => {
    const venue = makeVenue()
    const values = makeFormValues({
      instagram: 'https://instagram.com/newhandle',
    })

    const result = detectVenueChanges(values, venue)
    expect(result).toEqual({ instagram: 'https://instagram.com/newhandle' })
  })

  it('detects adding a new social link', () => {
    const venue = makeVenue()
    const values = makeFormValues({
      twitter: 'https://twitter.com/testvenue',
    })

    const result = detectVenueChanges(values, venue)
    expect(result).toEqual({ twitter: 'https://twitter.com/testvenue' })
  })

  it('sets social link to undefined when clearing', () => {
    const venue = makeVenue()
    const values = makeFormValues({ instagram: '' })

    const result = detectVenueChanges(values, venue)
    expect(result).toEqual({ instagram: undefined })
  })

  it('handles venue with no social object', () => {
    const venue = makeVenue({ social: undefined })
    const values = makeFormValues({
      instagram: '',
      facebook: '',
      twitter: '',
      youtube: '',
      spotify: '',
      soundcloud: '',
      bandcamp: '',
      website: '',
    })

    // All social fields default to '' when venue.social is undefined
    // Form values are also '' — no changes
    expect(detectVenueChanges(values, venue)).toBeNull()
  })

  it('detects website change with no social object', () => {
    const venue = makeVenue({ social: undefined })
    const values = makeFormValues({
      instagram: '',
      facebook: '',
      twitter: '',
      youtube: '',
      spotify: '',
      soundcloud: '',
      bandcamp: '',
      website: 'https://newsite.com',
    })

    const result = detectVenueChanges(values, venue)
    expect(result).toEqual({ website: 'https://newsite.com' })
  })

  it('works with Venue type (without upcoming_show_count)', () => {
    const venue: Venue = {
      id: 1,
      slug: 'v',
      name: 'Venue',
      address: null,
      city: 'Phoenix',
      state: 'AZ',
      verified: true,
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
    }
    const values = makeFormValues({
      name: 'Venue',
      address: '',
      city: 'Phoenix',
      state: 'AZ',
      zipcode: '',
      instagram: '',
      facebook: '',
      twitter: '',
      youtube: '',
      spotify: '',
      soundcloud: '',
      bandcamp: '',
      website: '',
    })

    expect(detectVenueChanges(values, venue)).toBeNull()
  })

  it('detects all social platform changes simultaneously', () => {
    const venue = makeVenue({
      social: {
        instagram: null,
        facebook: null,
        twitter: null,
        youtube: null,
        spotify: null,
        soundcloud: null,
        bandcamp: null,
        website: null,
      },
    })
    const values = makeFormValues({
      instagram: 'ig',
      facebook: 'fb',
      twitter: 'tw',
      youtube: 'yt',
      spotify: 'sp',
      soundcloud: 'sc',
      bandcamp: 'bc',
      website: 'ws',
    })

    const result = detectVenueChanges(values, venue)
    expect(result).toEqual({
      instagram: 'ig',
      facebook: 'fb',
      twitter: 'tw',
      youtube: 'yt',
      spotify: 'sp',
      soundcloud: 'sc',
      bandcamp: 'bc',
      website: 'ws',
    })
  })
})

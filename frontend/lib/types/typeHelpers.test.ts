import { describe, it, expect } from 'vitest'
import { getArtistLocation } from '@/features/artists'
import { getVenueLocation } from '@/features/venues'
import type { Artist } from '@/features/artists'
import type { Venue } from '@/features/venues'

describe('Type Helper Functions', () => {
  describe('getArtistLocation', () => {
    it('returns city and state when both are present', () => {
      const artist: Artist = {
        id: 1,
        slug: 'test-artist',
        name: 'Test Artist',
        city: 'Phoenix',
        state: 'AZ',
        bandcamp_embed_url: null,
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
        created_at: '2025-01-01T00:00:00Z',
        updated_at: '2025-01-01T00:00:00Z',
      }

      expect(getArtistLocation(artist)).toBe('Phoenix, AZ')
    })

    it('returns only city when state is null', () => {
      const artist: Artist = {
        id: 2,
        slug: 'city-artist',
        name: 'City Artist',
        city: 'Los Angeles',
        state: null,
        bandcamp_embed_url: null,
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
        created_at: '2025-01-01T00:00:00Z',
        updated_at: '2025-01-01T00:00:00Z',
      }

      expect(getArtistLocation(artist)).toBe('Los Angeles')
    })

    it('returns only state when city is null', () => {
      const artist: Artist = {
        id: 3,
        slug: 'state-artist',
        name: 'State Artist',
        city: null,
        state: 'California',
        bandcamp_embed_url: null,
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
        created_at: '2025-01-01T00:00:00Z',
        updated_at: '2025-01-01T00:00:00Z',
      }

      expect(getArtistLocation(artist)).toBe('California')
    })

    it('returns "Location Unknown" when both city and state are null', () => {
      const artist: Artist = {
        id: 4,
        slug: 'unknown-artist',
        name: 'Unknown Artist',
        city: null,
        state: null,
        bandcamp_embed_url: null,
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
        created_at: '2025-01-01T00:00:00Z',
        updated_at: '2025-01-01T00:00:00Z',
      }

      expect(getArtistLocation(artist)).toBe('Location Unknown')
    })

    it('filters out empty strings', () => {
      const artist: Artist = {
        id: 5,
        slug: 'empty-string-artist',
        name: 'Empty String Artist',
        city: '',
        state: 'AZ',
        bandcamp_embed_url: null,
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
        created_at: '2025-01-01T00:00:00Z',
        updated_at: '2025-01-01T00:00:00Z',
      }

      // Empty string is falsy, so it gets filtered out
      expect(getArtistLocation(artist)).toBe('AZ')
    })

    // PSY-558: country display rule. State + US-coded country suppresses the
    // country segment because "Phoenix, AZ" is US-implicit to local readers.
    // Everything else includes the country.
    const baseArtist = (
      overrides: Partial<Artist>,
    ): Artist => ({
      id: 100,
      slug: 's',
      name: 'n',
      city: null,
      state: null,
      country: null,
      bandcamp_embed_url: null,
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
      created_at: '2025-01-01T00:00:00Z',
      updated_at: '2025-01-01T00:00:00Z',
      ...overrides,
    })

    it('suppresses country when state is set and country is "USA"', () => {
      expect(
        getArtistLocation(baseArtist({ city: 'Phoenix', state: 'AZ', country: 'USA' })),
      ).toBe('Phoenix, AZ')
    })

    it('suppresses country when state is set and country is "US"', () => {
      expect(
        getArtistLocation(baseArtist({ city: 'Phoenix', state: 'AZ', country: 'US' })),
      ).toBe('Phoenix, AZ')
    })

    it('treats US suppression as case-insensitive', () => {
      expect(
        getArtistLocation(baseArtist({ city: 'Phoenix', state: 'AZ', country: 'usa' })),
      ).toBe('Phoenix, AZ')
    })

    it('includes non-US country with city only', () => {
      expect(
        getArtistLocation(baseArtist({ city: 'Melbourne', country: 'Australia' })),
      ).toBe('Melbourne, Australia')
    })

    it('includes non-US country with city + state', () => {
      expect(
        getArtistLocation(baseArtist({ city: 'London', state: 'England', country: 'UK' })),
      ).toBe('London, England, UK')
    })

    it('includes country when state is null even if country is US', () => {
      // No state => "USA" carries the locator load and should render.
      expect(
        getArtistLocation(baseArtist({ city: 'Phoenix', country: 'USA' })),
      ).toBe('Phoenix, USA')
    })

    it('handles Tokyo, Japan (no state, non-US country)', () => {
      expect(
        getArtistLocation(baseArtist({ city: 'Tokyo', country: 'Japan' })),
      ).toBe('Tokyo, Japan')
    })
  })

  describe('getVenueLocation', () => {
    it('returns city and state formatted correctly', () => {
      const venue: Venue = {
        id: 1,
        slug: 'the-rebel-lounge',
        name: 'The Rebel Lounge',
        address: '2303 E Indian School Rd',
        city: 'Phoenix',
        state: 'AZ',
        verified: true,
        created_at: '2025-01-01T00:00:00Z',
        updated_at: '2025-01-01T00:00:00Z',
      }

      expect(getVenueLocation(venue)).toBe('Phoenix, AZ')
    })

    it('handles different cities and states', () => {
      const venue: Venue = {
        id: 2,
        slug: 'some-venue',
        name: 'Some Venue',
        address: null,
        city: 'Tempe',
        state: 'Arizona',
        verified: false,
        created_at: '2025-01-01T00:00:00Z',
        updated_at: '2025-01-01T00:00:00Z',
      }

      expect(getVenueLocation(venue)).toBe('Tempe, Arizona')
    })

    it('handles venues with full state names', () => {
      const venue: Venue = {
        id: 3,
        slug: 'california-venue',
        name: 'California Venue',
        address: '123 Main St',
        city: 'Los Angeles',
        state: 'California',
        verified: true,
        created_at: '2025-01-01T00:00:00Z',
        updated_at: '2025-01-01T00:00:00Z',
      }

      expect(getVenueLocation(venue)).toBe('Los Angeles, California')
    })

    it('returns formatted location for venue with all optional fields', () => {
      const venue: Venue = {
        id: 4,
        slug: 'full-venue',
        name: 'Full Venue',
        address: '456 Oak Ave',
        city: 'Scottsdale',
        state: 'AZ',
        zipcode: '85251',
        verified: true,
        submitted_by: 1,
        social: {
          instagram: '@fullvenue',
          website: 'https://fullvenue.com',
        },
        created_at: '2025-01-01T00:00:00Z',
        updated_at: '2025-01-01T00:00:00Z',
      }

      expect(getVenueLocation(venue)).toBe('Scottsdale, AZ')
    })
  })
})

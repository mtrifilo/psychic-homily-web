import { describe, it, expect } from 'vitest'
import { getArtistLocation } from './artist'
import { getVenueLocation } from './venue'
import type { Artist } from './artist'
import type { Venue } from './venue'

describe('Type Helper Functions', () => {
  describe('getArtistLocation', () => {
    it('returns city and state when both are present', () => {
      const artist: Artist = {
        id: 1,
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
  })

  describe('getVenueLocation', () => {
    it('returns city and state formatted correctly', () => {
      const venue: Venue = {
        id: 1,
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

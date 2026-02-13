/**
 * Artist-related TypeScript types
 *
 * These types match the backend API response structures
 * from backend/internal/services/artist.go
 */

export interface ArtistSocial {
  instagram: string | null
  facebook: string | null
  twitter: string | null
  youtube: string | null
  spotify: string | null
  soundcloud: string | null
  bandcamp: string | null
  website: string | null
}

export interface Artist {
  id: number
  slug: string
  name: string
  state: string | null
  city: string | null
  bandcamp_embed_url: string | null
  social: ArtistSocial
  created_at: string
  updated_at: string
}

export interface ArtistEditRequest {
  name?: string
  city?: string
  state?: string
  instagram?: string
  facebook?: string
  twitter?: string
  youtube?: string
  spotify?: string
  soundcloud?: string
  bandcamp?: string
  website?: string
}

export interface ArtistSearchParams {
  query: string
}

export interface ArtistSearchResponse {
  artists: Artist[]
  count: number
}

/**
 * Get a formatted location string for an artist
 */
export const getArtistLocation = (artist: Artist): string => {
  const parts = [artist.city, artist.state].filter(Boolean)
  return parts.length > 0 ? parts.join(', ') : 'Location Unknown'
}

/**
 * Venue info in artist show response
 */
export interface ArtistShowVenue {
  id: number
  slug: string
  name: string
  city: string
  state: string
}

/**
 * Artist info in show response (simplified)
 */
export interface ArtistShowArtist {
  id: number
  slug: string
  name: string
}

/**
 * Show response for artist shows endpoint
 */
export interface ArtistShow {
  id: number
  slug: string
  title: string
  event_date: string
  price: number | null
  age_requirement: string | null
  venue: ArtistShowVenue | null
  artists: ArtistShowArtist[]
}

/**
 * Response from GET /artists/:id/shows
 */
export interface ArtistShowsResponse {
  shows: ArtistShow[]
  artist_id: number
  total: number
}

/**
 * Time filter options for artist shows
 */
export type ArtistTimeFilter = 'upcoming' | 'past' | 'all'


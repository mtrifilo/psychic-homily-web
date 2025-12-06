/**
 * Venue-related TypeScript types
 *
 * These types match the backend API response structures
 * from backend/internal/services/venue.go
 */

export interface Venue {
  id: number
  name: string
  address: string | null
  city: string
  state: string
  verified: boolean
  created_at: string
  updated_at: string
}

export interface VenueSearchParams {
  query: string
}

export interface VenueSearchResponse {
  venues: Venue[]
  count: number
}

/**
 * Get a formatted location string for a venue
 */
export const getVenueLocation = (venue: Venue): string => {
  return `${venue.city}, ${venue.state}`
}


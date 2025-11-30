/**
 * Show-related TypeScript types
 *
 * These types match the backend API response structures
 * from backend/internal/services/show.go
 */

export interface ShowArtistSocials {
  instagram?: string | null
  facebook?: string | null
  twitter?: string | null
  youtube?: string | null
}

export interface ArtistResponse {
  id: number
  name: string
  state?: string | null
  city?: string | null
  is_headliner?: boolean | null
  is_new_artist?: boolean | null
  socials: ShowArtistSocials
}

export interface VenueResponse {
  id: number
  name: string
  address?: string | null
  city: string
  state: string
  verified: boolean
}

export interface ShowResponse {
  id: number
  title: string
  event_date: string // ISO date string
  city?: string | null
  state?: string | null
  price?: number | null
  age_requirement?: string | null
  description?: string | null
  venues: VenueResponse[]
  artists: ArtistResponse[]
  created_at: string
  updated_at: string
}

export interface CursorPaginationMeta {
  next_cursor: string | null
  has_more: boolean
  limit: number
}

export interface UpcomingShowsResponse {
  shows: ShowResponse[]
  timezone: string
  pagination: CursorPaginationMeta
}

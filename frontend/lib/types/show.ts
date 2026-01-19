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

/**
 * Show approval status
 * - pending: awaiting admin review (contains unverified venue)
 * - approved: visible to public
 * - rejected: rejected by admin, not visible
 * - private: personal show, only visible to submitter
 */
export type ShowStatus = 'pending' | 'approved' | 'rejected' | 'private'

export interface ShowResponse {
  id: number
  title: string
  event_date: string // ISO date string
  city?: string | null
  state?: string | null
  price?: number | null
  age_requirement?: string | null
  description?: string | null
  status: ShowStatus
  submitted_by?: number
  rejection_reason?: string | null
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

// Admin response types
export interface PendingShowsResponse {
  shows: ShowResponse[]
  total: number
}

export interface RejectedShowsResponse {
  shows: ShowResponse[]
  total: number
}

export interface ApproveShowRequest {
  verify_venues: boolean
}

export interface RejectShowRequest {
  reason: string
}

// Saved shows (user's "My List") types
export interface SavedShowResponse extends ShowResponse {
  saved_at: string // ISO date string
}

export interface SavedShowsListResponse {
  shows: SavedShowResponse[]
  total: number
  limit: number
  offset: number
}

export interface SaveShowResponse {
  success: boolean
  message: string
}

export interface CheckSavedResponse {
  is_saved: boolean
}

// User's submitted shows response
export interface MySubmissionsResponse {
  shows: ShowResponse[]
  total: number
}

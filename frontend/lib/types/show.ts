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
  spotify?: string | null
  soundcloud?: string | null
  bandcamp?: string | null
  website?: string | null
}

export interface ArtistResponse {
  id: number
  slug: string
  name: string
  state?: string | null
  city?: string | null
  is_headliner?: boolean | null
  is_new_artist?: boolean | null
  bandcamp_embed_url?: string | null
  socials: ShowArtistSocials
}

export interface VenueResponse {
  id: number
  slug: string
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
  slug: string
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
  // Status flags (admin-controlled)
  is_sold_out: boolean
  is_cancelled: boolean
}

// Orphaned artist returned when a show edit removes an artist's only association
export interface OrphanedArtist {
  id: number
  name: string
  slug: string
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

// City with show count for filtering
export interface ShowCity {
  city: string
  state: string
  show_count: number
}

// Response for the show cities endpoint
export interface ShowCitiesResponse {
  cities: ShowCity[]
}

// Show report types
export type ShowReportType = 'cancelled' | 'sold_out' | 'inaccurate'
export type ShowReportStatus = 'pending' | 'dismissed' | 'resolved'

// Show info for report responses
export interface ShowReportShowInfo {
  id: number
  title: string
  slug: string
  event_date: string
  city?: string | null
  state?: string | null
}

// Show report response
export interface ShowReportResponse {
  id: number
  show_id: number
  report_type: ShowReportType
  details?: string | null
  status: ShowReportStatus
  admin_notes?: string | null
  reviewed_by?: number | null
  reviewed_at?: string | null
  created_at: string
  updated_at: string
  show?: ShowReportShowInfo
}

// Request to create a show report
export interface CreateShowReportRequest {
  report_type: ShowReportType
  details?: string
}

// Response for my-report endpoint
export interface MyShowReportResponse {
  report: ShowReportResponse | null
}

// Response for admin reports list
export interface ShowReportsListResponse {
  reports: ShowReportResponse[]
  total: number
}

// Request for admin actions on reports
export interface AdminReportActionRequest {
  notes?: string
}

// Request for resolving a report (extends AdminReportActionRequest)
export interface ResolveReportRequest extends AdminReportActionRequest {
  set_show_flag?: boolean
}

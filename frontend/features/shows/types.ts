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

export type SetType = 'headliner' | 'opener' | 'performer' | 'special_guest'

export interface ArtistResponse {
  id: number
  slug: string
  name: string
  state?: string | null
  city?: string | null
  is_headliner?: boolean | null
  set_type: SetType
  position: number
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
  /** IANA timezone for rendering this show's time in venue-local time (PSY-985/986). Null until backfilled. */
  timezone?: string | null
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
  ticket_url?: string | null
  image_url?: string | null
  status: ShowStatus
  submitted_by?: number
  rejection_reason?: string | null
  rejection_category?: string | null
  venues: VenueResponse[]
  artists: ArtistResponse[]
  created_at: string
  updated_at: string
  // Status flags (admin-controlled)
  is_sold_out: boolean
  is_cancelled: boolean
  // Discovery source fields
  source?: string
  source_venue?: string
  scraped_at?: string
  // Duplicate detection context
  duplicate_of_show_id?: number
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
  category?: string
}

export type RejectionCategory = 'non_music' | 'duplicate' | 'bad_data' | 'past_event' | 'other'

export interface BatchShowError {
  show_id: number
  error: string
}

export interface BatchApproveResponse {
  approved: number
  errors: BatchShowError[]
}

export interface BatchRejectResponse {
  rejected: number
  errors: BatchShowError[]
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
  // Geocoded city centroid (PSY-981, same offline GeoNames source as PSY-985
  // venue coords). Omitted by the backend when the geocoder can't resolve the
  // city; the client then falls back to exact city-name matching for geo.
  latitude?: number
  longitude?: number
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

// Calendar feed types
export interface CalendarTokenStatusResponse {
  has_token: boolean
  created_at?: string // ISO date string
}

export interface CalendarTokenCreateResponse {
  token: string
  feed_url: string
  /** Atom activity feed for followed artists (PSY-1505); same personal token. */
  follows_feed_url: string
  created_at: string // ISO date string
}

export interface CalendarTokenDeleteResponse {
  success: boolean
  message: string
}

// Public save-count types. The count is an aggregate visible to everyone;
// is_saved reflects the requesting user and is always false when anonymous.
export interface ShowSaveCount {
  show_id: number
  save_count: number
  is_saved: boolean
}

export interface SaveCountEntry {
  save_count: number
  is_saved: boolean
}

export interface BatchSaveCountsResponse {
  saves: Record<string, SaveCountEntry>
}

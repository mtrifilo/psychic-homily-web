/**
 * Show-related TypeScript types
 *
 * These types match the backend API response structures
 * from backend/internal/services/show.go
 */

import type { components } from '@/types/api'

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

/**
 * Curated bill role for one act on one show (show_artists.set_type).
 *
 * DERIVED from the generated OpenAPI enum rather than hand-listed, so the
 * vocabulary has exactly one source of truth: `SetTypeVocabulary()` in
 * backend/internal/services/contracts/set_type.go, which the `enum` tag on the
 * show create/update body publishes into types/api.d.ts. Regenerating types is
 * a CI gate, so a value added on the server widens this union automatically --
 * and the exhaustive SET_TYPE_LABELS record then fails to compile until the
 * form is taught the new role. That compile error is the whole point: a
 * hand-written mirror could silently fall behind, and the show form sends
 * set_type back on every save, so a role this build cannot represent would be
 * quietly overwritten.
 *
 * Deliberately reads the REQUEST schema: only that one carries the enum
 * (responses type set_type as a bare string), and the request enum is the
 * authoritative contract for what may be written.
 *
 * 'performer' is the NEUTRAL DEFAULT and means "on the bill, slot unknown".
 * It is what every uncurated row holds, so it must never be rendered as a
 * role. Every other value is an assertion somebody actually made.
 */
export type SetType = NonNullable<
  components['schemas']['Artist']['set_type']
>

/**
 * Minimal label reference rendered next to an artist on the show bill.
 *
 * DERIVED from the generated OpenAPI schema rather than hand-written, for the
 * same reason `SetType` above is: a hand-written mirror of a backend struct
 * can fall behind it silently, and regenerating types is a CI gate.
 *
 * `slug` can be an empty string: `labels.slug` is nullable in the database and
 * the backend flattens null to "". Callers must treat empty as "no label page
 * to link to" rather than building `/labels/`.
 */
export type ShowArtistLabel = components['schemas']['ShowArtistLabel']

export interface ArtistResponse {
  id: number
  slug: string
  name: string
  state?: string | null
  city?: string | null
  country?: string | null
  is_headliner?: boolean | null
  set_type: SetType
  position: number
  /**
   * Labels this artist records for, name-ascending. Populated only by the
   * show-DETAIL reads (`GET /shows/{id}`); list endpoints omit the key rather
   * than pay two queries per show for a field their cards never render.
   *
   * Absent (undefined) means "not looked up"; present-but-empty means "looked
   * up, artist is unsigned". Keep that distinction: rendering an absent key as
   * "no labels" is the same pixels, but a consumer that needs to know whether
   * the lookup happened (a "add a label" prompt, say) would be reading a lie.
   */
  labels?: ShowArtistLabel[]
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
  /** Room capacity (PSY-1179/1682). Null when unknown. Not redacted for unverified venues. */
  capacity?: number | null
  /**
   * The venue's HOUSE DEFAULT age rule (PSY-1682), free text ("all ages",
   * "17+", "21+"). The show's own `age_requirement` is the PER-EVENT OVERRIDE
   * and wins wherever both are set. Null means unknown, not "all ages".
   */
  age_policy?: string | null
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
  // Display times, null when unannounced (the common case). The backend always
  // emits both keys; these are optional here only because every field in this
  // hand-written mirror is, and tightening two of them alone would be the
  // inconsistency. types/api.d.ts is the strict generated contract.
  doors_at?: string | null
  music_at?: string | null
  city?: string | null
  state?: string | null
  price?: number | null
  age_requirement?: string | null
  description?: string | null
  ticket_url?: string | null
  image_url?: string | null
  status: ShowStatus
  submitted_by?: number
  /**
   * Resolved display identity of `submitted_by`, so the provenance byline can
   * credit the submitter without a second round trip (PSY-1866).
   *
   * DETAIL READS ONLY (`GET /shows/{id|slug}`). List payloads omit both keys.
   *
   * Absence of `submitted_by_name` is deliberately opaque: it means "this show
   * has no submitter, OR the payload is a list row, OR the backend's privacy
   * gates withheld the credit". The backend fails closed — it drops the credit
   * for a contributor who set `contributions: hidden`, and for an account whose
   * only resolvable name would come from its email local-part. Do NOT try to
   * distinguish those cases or substitute a placeholder; render no credit.
   *
   * `submitted_by_username` is the `/users/:username` slug. It is absent when
   * the account has no username AND when the profile is private (linking there
   * would be a dead link), so a name with no username must render as unlinked
   * text. `UserAttribution` already gates on exactly that.
   */
  submitted_by_name?: string | null
  submitted_by_username?: string | null
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
  /** Full matching-set size under the current filters (not just this page). */
  total: number
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

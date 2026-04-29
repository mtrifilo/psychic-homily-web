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
  description?: string | null
  social: ArtistSocial
  created_at: string
  updated_at: string
}

export interface ArtistEditRequest {
  name?: string
  city?: string
  state?: string
  description?: string
  instagram?: string
  facebook?: string
  twitter?: string
  youtube?: string
  spotify?: string
  soundcloud?: string
  bandcamp?: string
  website?: string
}

export interface ArtistCity {
  city: string
  state: string
  artist_count: number
}

export interface ArtistCitiesResponse {
  cities: ArtistCity[]
}

export interface ArtistListItem extends Artist {
  upcoming_show_count: number
  /**
   * Most recent past approved show date (ISO string). Only populated when the
   * backend is running in evergreen mode — i.e. when the list was requested
   * with a tag filter (PSY-495 Bandcamp model). Undefined on the default
   * activity-gated /artists landing because those artists always have at
   * least one upcoming show.
   */
  last_show_date?: string | null
}

export interface ArtistsListResponse {
  artists: ArtistListItem[]
  count: number
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

// Artist report types
export type ArtistReportType = 'inaccurate' | 'removal_request'
export type ArtistReportStatus = 'pending' | 'dismissed' | 'resolved'

// Artist info for report responses
export interface ArtistReportArtistInfo {
  id: number
  name: string
  slug: string
}

// Artist report response
export interface ArtistReportResponse {
  id: number
  artist_id: number
  report_type: ArtistReportType
  details?: string | null
  status: ArtistReportStatus
  admin_notes?: string | null
  reviewed_by?: number | null
  reviewed_at?: string | null
  created_at: string
  updated_at: string
  artist?: ArtistReportArtistInfo
}

// Request to create an artist report
export interface CreateArtistReportRequest {
  report_type: ArtistReportType
  details?: string
}

// Response for my-report endpoint
export interface MyArtistReportResponse {
  report: ArtistReportResponse | null
}

// Response for admin artist reports list
export interface ArtistReportsListResponse {
  reports: ArtistReportResponse[]
  total: number
}

// Artist alias
export interface ArtistAlias {
  id: number
  artist_id: number
  alias: string
  created_at: string
}

// Response for artist aliases endpoint
export interface ArtistAliasesResponse {
  aliases: ArtistAlias[]
  count: number
}

// Artist graph types
export interface ArtistGraphNode {
  id: number
  name: string
  slug: string
  city?: string
  state?: string
  image_url?: string
  upcoming_show_count: number
}

export interface ArtistGraphLink {
  source_id: number
  target_id: number
  type: string
  score: number
  votes_up: number
  votes_down: number
  detail?: Record<string, unknown>
}

export interface ArtistGraph {
  center: ArtistGraphNode
  nodes: ArtistGraphNode[]
  links: ArtistGraphLink[]
  user_votes?: Record<string, string> // "sourceID-targetID-type" -> "up"/"down"
}

// Bill composition (PSY-364) — derived from show_artists.position + set_type.
export interface BillStats {
  total_shows: number
  headliner_count: number
  opener_count: number
}

export interface BillCoArtist {
  artist: ArtistGraphNode
  shared_count: number
  last_shared: string // ISO date "2026-03-01"
}

export interface ArtistBillComposition {
  artist: ArtistGraphNode
  stats: BillStats
  opens_with: BillCoArtist[]
  closes_with: BillCoArtist[]
  graph: ArtistGraph
  below_threshold: boolean
  time_filter_months: number // 0 = all-time
}

// Merge artist result
export interface MergeArtistResult {
  canonical_artist_id: number
  merged_artist_id: number
  merged_artist_name: string
  shows_moved: number
  releases_moved: number
  labels_moved: number
  festivals_moved: number
  relationships_moved: number
  bookmarks_moved: number
  alias_created: boolean
}

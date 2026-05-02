/**
 * Venue-related TypeScript types
 *
 * These types match the backend API response structures
 * from backend/internal/services/venue.go
 */

import type { ArtistResponse } from '@/features/shows'

export interface Venue {
  id: number
  slug: string
  name: string
  address: string | null
  city: string
  state: string
  zipcode?: string | null
  description?: string | null
  /** Optional venue photo URL (PSY-521). */
  image_url?: string | null
  verified: boolean
  submitted_by?: number | null
  social?: {
    instagram?: string | null
    facebook?: string | null
    twitter?: string | null
    youtube?: string | null
    spotify?: string | null
    soundcloud?: string | null
    bandcamp?: string | null
    website?: string | null
  }
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
 * Venue with upcoming show count for the venues list
 */
export interface VenueWithShowCount extends Venue {
  upcoming_show_count: number
}

/**
 * Response for the venues list endpoint
 */
export interface VenuesListResponse {
  venues: VenueWithShowCount[]
  total: number
  limit: number
  offset: number
}

/**
 * Show response in the venue shows endpoint
 */
export interface VenueShow {
  id: number
  slug: string
  title: string
  event_date: string
  city: string | null
  state: string | null
  price: number | null
  age_requirement: string | null
  artists: ArtistResponse[]
}

/**
 * Response for the venue shows endpoint
 */
export interface VenueShowsResponse {
  shows: VenueShow[]
  venue_id: number
  total: number
}

/**
 * City with venue count for filtering
 */
export interface VenueCity {
  city: string
  state: string
  venue_count: number
}

/**
 * Response for the venue cities endpoint
 */
export interface VenueCitiesResponse {
  cities: VenueCity[]
}

/**
 * Get a formatted location string for a venue
 */
export const getVenueLocation = (venue: Venue): string => {
  return `${venue.city}, ${venue.state}`
}

// ============================================================================
// Venue Editing Types
// ============================================================================

/**
 * Status of a pending venue edit
 */
/**
 * Request body for PUT /venues/{id} (admin-only direct update).
 * Non-admin users go through the unified suggest-edit flow instead.
 */
export interface VenueEditRequest {
  name?: string
  address?: string
  city?: string
  state?: string
  zipcode?: string
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

// ============================================================================
// Admin Unverified Venues Types
// ============================================================================

/**
 * Unverified venue awaiting admin verification
 */
export interface UnverifiedVenue {
  id: number
  slug: string
  name: string
  address: string | null
  city: string
  state: string
  zipcode: string | null
  submitted_by: number | null
  created_at: string
  show_count: number
}

/**
 * Response for admin listing unverified venues
 */
export interface UnverifiedVenuesResponse {
  venues: UnverifiedVenue[]
  total: number
}

// ============================================================================
// Favorite Venues Types
// ============================================================================

/**
 * Favorite venue with metadata
 */
export interface FavoriteVenueResponse {
  id: number
  slug: string
  name: string
  address: string | null
  city: string
  state: string
  verified: boolean
  favorited_at: string
  upcoming_show_count: number
}

/**
 * Response for listing favorite venues
 */
export interface FavoriteVenuesListResponse {
  venues: FavoriteVenueResponse[]
  total: number
  limit: number
  offset: number
}

/**
 * Response for checking if a venue is favorited
 */
export interface CheckFavoritedResponse {
  is_favorited: boolean
}

/**
 * Response for favoriting/unfavoriting a venue
 */
export interface FavoriteVenueActionResponse {
  success: boolean
  message: string
}

/**
 * Show from a favorite venue (includes venue info)
 */
export interface FavoriteVenueShow {
  id: number
  slug: string
  title: string
  event_date: string
  city: string | null
  state: string | null
  price: number | null
  age_requirement: string | null
  venue_id: number
  venue_name: string
  venue_slug: string
  artists: ArtistResponse[]
}

/**
 * Response for getting shows from favorite venues
 */
export interface FavoriteVenueShowsResponse {
  shows: FavoriteVenueShow[]
  total: number
  limit: number
  offset: number
  timezone: string
}

// ============================================================================
// Venue Genre Profile Types
// ============================================================================

export interface VenueGenreCount {
  tag_id: number
  name: string
  slug: string
  count: number
}

export interface VenueGenreResponse {
  genres: VenueGenreCount[]
}

// ============================================================================
// Venue Bill Network (PSY-365) — venue-rooted co-bill graph
// ============================================================================
//
// Mirrors the backend `contracts.VenueBillNetworkResponse` 1:1. The shared
// frontend `ForceGraphView` consumes the same node/cluster/link shape as the
// scene graph, so the field names + types stay aligned with `SceneGraphNode`
// etc. — the only addition is `at_venue_show_count` (per-node) and the
// venue-window metadata (per-response).

/**
 * Time-window filter for the venue bill network. Mirrors the backend's
 * normalized `Window` field on the response — the frontend reuses the same
 * vocabulary on input (query param) and output (response label).
 */
export type VenueBillNetworkWindow = 'all' | '12m' | 'year'

export interface VenueBillNetworkInfo {
  id: number
  slug: string
  name: string
  city?: string
  state?: string
  artist_count: number
  edge_count: number
  show_count: number
  /** Backend-normalized label: "all_time", "last_12m", or "year". */
  window: 'all_time' | 'last_12m' | 'year'
  /** Populated only when window === "year". */
  year?: number
}

export interface VenueBillNetworkCluster {
  id: string
  label: string
  size: number
  color_index: number
}

export interface VenueBillNetworkNode {
  id: number
  name: string
  slug: string
  city?: string
  state?: string
  upcoming_show_count: number
  cluster_id: string
  is_isolate: boolean
  /** Number of approved shows this artist has played at the venue, in window. */
  at_venue_show_count: number
}

export interface VenueBillNetworkLink {
  source_id: number
  target_id: number
  type: string
  score: number
  detail?: Record<string, unknown>
  is_cross_cluster: boolean
}

export interface VenueBillNetworkResponse {
  venue: VenueBillNetworkInfo
  clusters: VenueBillNetworkCluster[]
  nodes: VenueBillNetworkNode[]
  links: VenueBillNetworkLink[]
}

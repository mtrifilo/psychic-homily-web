/**
 * Venue-related TypeScript types
 *
 * These types match the backend API response structures
 * from backend/internal/services/venue.go
 */

import type { ArtistResponse } from '@/features/shows'
import { formatLocation } from '@/lib/formatLocation'

export interface Venue {
  id: number
  slug: string
  name: string
  address: string | null
  city: string
  state: string
  /** IANA timezone resolved from the venue's location (PSY-985). Null until backfilled. */
  timezone?: string | null
  /**
   * City-centroid coordinates, geocoded offline from city/state. Present for
   * every venue the geocoder recognizes — this is the coarse position, and the
   * ONLY position an unverified venue ever has on a map.
   */
  latitude?: number | null
  longitude?: number | null
  /**
   * Street-precise coordinates (PSY-1536). Served ONLY for verified venues
   * whose geocode still matches their current address — the API omits them for
   * everyone else, which is the privacy gate protecting DIY and house venues
   * from being street-mapped before human review. Absent means "pin at the
   * centroid", never "look the address up some other way".
   */
  street_latitude?: number | null
  street_longitude?: number | null
  /** How the street geocode was resolved: rooftop | interpolated | city. */
  geocode_precision?: string | null
  zipcode?: string | null
  /**
   * Room capacity (PSY-1179). Deliberately NOT redacted for unverified venues
   * (the backend's buildVenueResponse notes it isn't sensitive, unlike
   * address/zipcode) — so a capacity may be present on a venue whose address
   * is withheld.
   */
  capacity?: number | null
  /**
   * The venue's HOUSE DEFAULT age rule (PSY-1682), free text mirroring the
   * show-level `age_requirement` vocabulary ("all ages", "17+", "21+"). A
   * show's own `age_requirement` is the PER-EVENT OVERRIDE and always wins;
   * this is the default it departs from. Null means unknown, not "all ages".
   * Like capacity, not redacted for unverified venues.
   */
  age_policy?: string | null
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
  /**
   * Freshness / attribution stamp (PSY-1542). Present on venue detail and on
   * the Atlas city-scoped list (`includeRail`), absent on the venue browse
   * list, which doesn't render it and shouldn't pay for the aggregations.
   */
  provenance?: VenueProvenance
}

/**
 * How fresh a venue listing is and who says so.
 *
 * A crowdsourced venue map dies of staleness nobody can see, so this exists to
 * make the age of a listing visible. Every number is an honest aggregate over
 * rows the backend already keeps — a segment whose count is 0 is OMITTED from
 * the rendered line rather than shown as "0 edits", because an absent claim
 * reads better than an empty one.
 */
export interface VenueProvenance {
  /** The venue row's `updated_at` — the last write of any kind. */
  updated_at: string
  /**
   * APPROVED community edits. Pending and rejected proposals are excluded:
   * they never changed what you are looking at.
   */
  edit_count: number
  /** Distinct people behind `edit_count`. */
  contributor_count: number
  /** Distinct people who have tapped "confirm info". */
  confirmation_count: number
  /** Most recent confirmation; absent when there are none. */
  last_confirmed_at?: string
  /**
   * Which kinds of source touched this venue: `'ingest'` and/or
   * `'community'`. Frequently EMPTY — `'ingest'` is only emitted when the
   * backend's `venues.data_source` column is populated, and no live writer
   * fills it today. Render the segment only when the array is non-empty.
   */
  sources: string[]
}

/** Response for POST /venues/{id}/confirm. */
export interface VenueConfirmationResponse {
  confirmation_count: number
  last_confirmed_at?: string
  /** Always true on success, including the idempotent repeat confirm. */
  viewer_has_confirmed: boolean
}

export interface VenueSearchResponse {
  venues: Venue[]
  count: number
}

/**
 * Venue with upcoming show count for the venues list.
 *
 * The fields below `upcoming_show_count` are the Atlas city-view rail's row
 * payload (PSY-1539). All optional: the backend fills them best-effort, and a
 * venue with nothing booked legitimately has none of them.
 */
export interface VenueWithShowCount extends Venue {
  upcoming_show_count: number
  /**
   * The <=7-day slice of `upcoming_show_count`. Drives the "Next 7 days" chip
   * and the rail's header stat.
   *
   * ROLLING from now, not a Monday-to-Sunday week — which is why both of those
   * say "next 7 days" rather than "this week" (PSY-1732). Same rule and same
   * trap as `SceneListItem.shows_this_week`, at venue scope instead of scene.
   */
  shows_this_week?: number
  /** Soonest upcoming show, `YYYY-MM-DD`, ALREADY in the venue's timezone. */
  next_show_date?: string
  /** That show's own title — empty for most shows; fall back to the artists. */
  next_show_title?: string
  /** That show's bill in position order (the title fallback). */
  next_show_artists?: string[]
  /** Dominant genre-family key, matching `GENRE_FAMILIES`. */
  dominant_genre?: string
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
  /** Show was called off. Renders as a destructive badge on the bill. */
  is_cancelled: boolean
  /** No tickets left. Renders as an outline badge on the bill. */
  is_sold_out: boolean
  artists: ArtistResponse[]
}

/**
 * Response for the venue shows endpoint.
 *
 * `limit`/`offset`/`year` echo the query back, so a reader of the response
 * can tell which slice it is without re-deriving it from the request.
 */
export interface VenueShowsResponse {
  shows: VenueShow[]
  venue_id: number
  /** Rows matching the time filter AND year, across every page. */
  total: number
  limit: number
  offset: number
  /** Year filter applied, or 0 for "every year". */
  year: number
}

/**
 * What a venue's show rows need in order to be placed on a calendar.
 *
 * Rendering a show's date, time or month is always done in the VENUE's zone,
 * never the reader's (PSY-985/986). `venueTimezone` is the resolved IANA zone
 * and wins when it is known; `venueState` is the fallback for venues that
 * predate the backfill, and is itself overridden by a row's own `state`.
 */
export interface VenueShowZone {
  venueState: string
  venueTimezone?: string | null
}

/** One bar of the past-shows year histogram (PSY-1753). */
export interface VenueShowYearCount {
  /** Venue-local calendar year. */
  year: number
  /** Shows at this venue in that year, within the requested time filter. */
  count: number
}

/** Response for `GET /venues/{id}/shows/years`. */
export interface VenueShowYearsResponse {
  venue_id: number
  /** Time filter the counts were taken under. */
  time_filter: string
  /** Years with at least one show, newest first. Never contains a zero count. */
  years: VenueShowYearCount[]
}

/**
 * One bar of the past-shows MONTH histogram (PSY-1769).
 *
 * Bucketed on the venue's own calendar, server-side — the year and month are
 * already decided, and re-reading them through a timezone would be wrong.
 */
export interface VenueShowMonthCount {
  /** Venue-local calendar year. */
  year: number
  /** Venue-local calendar month, 1-12. */
  month: number
  /** Shows at this venue in that month, within the requested time filter. */
  count: number
}

/** Response for `GET /venues/{id}/shows/months`. */
export interface VenueShowMonthsResponse {
  venue_id: number
  /** Time filter the counts were taken under. */
  time_filter: string
  /** Months with at least one show, newest first. Never contains a zero count. */
  months: VenueShowMonthCount[]
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
 * Get a formatted location string for a venue.
 *
 * Delegates to the shared `formatLocation` helper (PSY-780) so empty/missing
 * state no longer leaves a trailing ", " in the UI. The `Venue` model does
 * not currently carry a `country` field, so the PSY-558 country-suppression
 * rule is a no-op here today. `Venue` is passed directly (structural typing)
 * so when `country` is added to the model the field will flow through this
 * helper without further changes here.
 */
export const getVenueLocation = (venue: Venue): string => formatLocation(venue)

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
  /**
   * Full in-window artist count BEFORE the backend's 150-node cap, plus
   * whether the cap bit — the scene graph's "not a silent cap" convention,
   * so "top N of M" copy is possible without an API change.
   */
  artist_total: number
  roster_truncated: boolean
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

/**
 * Artist-related TypeScript types
 *
 * These types match the backend API response structures
 * from backend/internal/services/artist.go
 */

import { formatLocation } from '@/lib/formatLocation'
import type { EntityReportResponse } from '@/features/contributions/types'

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

/**
 * Five at-a-glance counts surfaced in the artist detail page sidebar (PSY-639).
 * Populated only on single-artist detail responses (GetArtist /
 * GetArtistBySlug); undefined on list / search / mutation responses.
 *
 * `shows_tracked` counts past + future shows. `active_since` was considered
 * and dropped — most artists lack the signals to derive it accurately.
 */
export interface ArtistStats {
  releases: number
  labels: number
  shows_tracked: number
  similar_artists: number
  festival_appearances: number
}

export interface Artist {
  id: number
  slug: string
  name: string
  state: string | null
  city: string | null
  /**
   * Optional country (PSY-558). Surfaced in the location pill conditionally —
   * see `getArtistLocation` for the display rule (US + state set hides "USA").
   */
  country?: string | null
  bandcamp_embed_url: string | null
  description?: string | null
  /** Optional artist photo URL (PSY-521). */
  image_url?: string | null
  /** Image provider + deep linkback for attribution (PSY-1175). */
  image_source?: string | null
  image_source_url?: string | null
  /** CC license + photographer for a Commons-sourced photo (PSY-1232). */
  image_license?: string | null
  image_author?: string | null
  social: ArtistSocial
  created_at: string
  updated_at: string
  /** Populated by detail-page lookups (PSY-639). Undefined on list rows. */
  stats?: ArtistStats
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
  /**
   * Artists matching the filters across EVERY page, not the length of this
   * one. The pager sizes itself from it, and the count line reports it.
   */
  total: number
  limit: number
  offset: number
}

export interface ArtistSearchResponse {
  artists: Artist[]
  count: number
}

/**
 * Get a formatted location string for an artist (PSY-558 display rule).
 *
 * Thin wrapper around the shared `formatLocation` helper — see
 * `lib/formatLocation.ts` for the full rule. Kept as a named export so
 * artist call sites and tests don't need to import from `lib/` directly
 * and the structural type is conveniently labeled "artist" at the call site.
 */
export const getArtistLocation = (
  artist: { city?: string | null; state?: string | null; country?: string | null },
): string => formatLocation(artist)

/**
 * Venue info in artist show response
 */
export interface ArtistShowVenue {
  id: number
  slug: string
  name: string
  city: string
  state: string
  /** IANA timezone for rendering this show's time in venue-local time (PSY-985/986). Null until backfilled. */
  timezone?: string | null
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
  /** The advance/door pair; see VenueShow.price for why both are served. */
  price: number | null
  door_price: number | null
  age_requirement: string | null
  /** Show was called off. Renders as a destructive badge on the bill. */
  is_cancelled: boolean
  /** No tickets left. Renders as an outline badge on the bill. */
  is_sold_out: boolean
  /**
   * Null when the show has no venue link, or when its venue row could not be
   * resolved. The wire schema types this non-nullable, but the service builds
   * it from two separate lookups and leaves it nil when either misses, so rows
   * with no venue do reach the client.
   */
  venue: ArtistShowVenue | null
  artists: ArtistShowArtist[]
}

/**
 * Response from GET /artists/:id/shows
 *
 * `limit`/`offset`/`year` echo the query back, so a reader of the response can
 * tell which slice it is without re-deriving it from the request.
 */
export interface ArtistShowsResponse {
  shows: ArtistShow[]
  artist_id: number
  /** Rows matching the time filter AND year, across every page. */
  total: number
  limit: number
  offset: number
  /** Year filter applied, or 0 for "every year". */
  year: number
}

/** One bar of the past-shows year histogram (PSY-1754). */
export interface ArtistShowYearCount {
  /** Venue-local calendar year. */
  year: number
  /** Shows the artist played that year, within the requested time filter. */
  count: number
}

/** Response for `GET /artists/{id}/shows/years`. */
export interface ArtistShowYearsResponse {
  artist_id: number
  /** Time filter the counts were taken under. */
  time_filter: string
  /** Years with at least one show, newest first. Never contains a zero count. */
  years: ArtistShowYearCount[]
}

/**
 * One bar of the past-shows MONTH histogram (PSY-1842).
 *
 * Bucketed on EACH SHOW'S OWN venue's calendar, server-side — an artist's rows
 * span venues, so there is no single zone to read them in, and the year and
 * month are already decided by the time they arrive. Re-reading them through a
 * timezone here would be wrong.
 */
export interface ArtistShowMonthCount {
  /** Venue-local calendar year. */
  year: number
  /** Venue-local calendar month, 1-12. */
  month: number
  /** Shows the artist played that month, within the requested time filter. */
  count: number
}

/** Response for `GET /artists/{id}/shows/months`. */
export interface ArtistShowMonthsResponse {
  artist_id: number
  /** Time filter the counts were taken under. */
  time_filter: string
  /** Months with at least one show, newest first. Never contains a zero count. */
  months: ArtistShowMonthCount[]
}

/**
 * Time filter options for artist shows
 */
export type ArtistTimeFilter = 'upcoming' | 'past' | 'all'

/**
 * Response for `GET /artists/{id}/my-report`.
 *
 * PSY-1633: artists have no report shape of their own any more. Reporting an
 * artist goes through the generic entity pipeline (`useReportEntity`), lands in
 * `entity_reports`, and comes back as an `EntityReportResponse` — which is what
 * the endpoint had been returning all along while this feature's types claimed
 * an `artist_id`/`report_type` shape that no longer existed on the wire.
 */
export interface MyArtistReportResponse {
  report: EntityReportResponse | null
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
  /** Selecting this node opens a playable embed — drives the shared violet
   * playable-marker ring. Mirrors the backend flag. */
  has_playable_audio?: boolean
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

// ============================================================================
// Artist graph card (PSY-1345)
// ============================================================================

/**
 * Node-select summary card for graph surfaces — mirrors
 * backend contracts.ArtistGraphCard. `next_show`/`radio` are null (not
 * omitted) when absent; `labels` is always an array.
 */
export interface ArtistGraphCard {
  id: number
  name: string
  slug: string
  city: string | null
  state: string | null
  /**
   * Playable audio (PSY-1302): the artist's Bandcamp embed URL and Spotify
   * link, so the node-select card can play a sample without leaving the graph
   * (the same MusicEmbed the Atlas scene preview uses). Both null ⟹ no player.
   */
  bandcamp_embed_url: string | null
  spotify: string | null
  next_show: ArtistGraphCardShow | null
  labels: ArtistGraphCardLabel[]
  radio: ArtistGraphCardRadio | null
  connections: ArtistGraphCardConnections
}

export interface ArtistGraphCardShow {
  id: number
  event_date: string
  venue_name: string
  venue_city: string
  venue_state: string
  /** IANA zone (PSY-985) — render the date in venue-local time. */
  venue_timezone: string | null
}

export interface ArtistGraphCardLabel {
  name: string
  slug: string
}

export interface ArtistGraphCardRadio {
  stations: string[]
  play_count: number
}

export interface ArtistGraphCardConnections {
  bills: number
  similar: number
  members: number
  radio: number
  shared_labels: number
}

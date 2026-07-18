/**
 * Wire-shape types for the /explore landing read endpoints.
 * Mirrors `backend/internal/services/contracts/explore.go`.
 *
 * Backend uses Huma — the JSON wire payload is the value of the Go-side
 * `Body` field; there is NO `body` envelope key. These types describe
 * the flat shape clients receive directly.
 */

// ──────────────────────────────────────────────
// GET /explore/upcoming-shows
// ──────────────────────────────────────────────

/**
 * Single row on the Upcoming Shows list. Headliner is denormalized for
 * tile rendering; richer bill detail lives on the show-detail page.
 */
export interface ExploreUpcomingShowItem {
  id: number
  slug: string
  title: string
  event_date: string
  /** Show's headliner name (set_type='headliner' or first artist by position). */
  headliner_name: string
  /** First associated venue's name/city/state. */
  venue_name: string
  venue_city: string
  venue_state: string
  /** Show-level city/state overrides (rare — backend prefers venue values). */
  city?: string | null
  state?: string | null
}

export interface ExploreUpcomingShowsResponse {
  shows: ExploreUpcomingShowItem[]
  total: number
  limit: number
  offset: number
}

// ──────────────────────────────────────────────
// GET /explore/shuffle-target
// ──────────────────────────────────────────────

/**
 * Random artist drawn from the ±90-day show pool. All fields are
 * nullable: a cold-start database with no qualifying artists returns
 * an all-null response with HTTP 200 (graceful empty state).
 */
export type { RandomArtistTargetResponse as ExploreShuffleTargetResponse } from '@/features/discovery/types'

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
// GET /explore/featured
// ──────────────────────────────────────────────

/**
 * Featured Bill referent. `curator_note_html` is the pre-rendered HTML
 * (goldmark + bluemonday sanitized) — render with
 * `dangerouslySetInnerHTML` directly; the renderer is the canonical
 * sanitizer shared with comments + collections.
 */
export interface ExploreFeaturedBill {
  id: number
  slug: string
  title: string
  event_date: string
  headliner_name: string
  venue_name: string
  venue_city: string
  venue_state: string
  image_url?: string | null
  curator_note?: string | null
  curator_note_html?: string
}

export interface ExploreFeaturedCollection {
  id: number
  slug: string
  title: string
  description?: string
  description_html?: string
  cover_image_url?: string | null
  curator_note?: string | null
  curator_note_html?: string
}

/**
 * Both fields are nullable. When both are null the entire Featured
 * section collapses; each can be null independently (admin can curate
 * just one slot type).
 */
export interface ExploreFeaturedResponse {
  bill: ExploreFeaturedBill | null
  collection: ExploreFeaturedCollection | null
}

// ──────────────────────────────────────────────
// GET /explore/shuffle-target
// ──────────────────────────────────────────────

/**
 * Random artist drawn from the ±90-day show pool. All fields are
 * nullable: a cold-start database with no qualifying artists returns
 * an all-null response with HTTP 200 (graceful empty state).
 */
export interface ExploreShuffleTargetResponse {
  artist_id: number | null
  artist_slug: string | null
  artist_name: string | null
}

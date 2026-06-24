/**
 * Wire-shape types for the admin music-link suggestion review queue
 * (PSY-1199/1207). Mirrors the LOCKED backend contract
 * `backend/internal/services/contracts/link_suggestion.go`.
 *
 * The list endpoint surfaces ONLY `pending` rows, high-confidence first
 * (the server orders `high` before `review`, then `id ASC`). Accept writes
 * the link (Spotify → social.spotify embed; Bandcamp → social.bandcamp +
 * the PSY-1190 profile→embed resolver, which runs server-side async).
 * Reject just marks the row. Both stamp the reviewer and are idempotent on
 * replay; a re-review with a *different* verdict is a 409.
 */

/** Streaming platform a suggestion targets. */
export type LinkSuggestionPlatform = 'bandcamp' | 'spotify'

/**
 * Region confidence tier (PSY-1191 semantics, carried through the sweep):
 * `high` = the MusicBrainz candidate's geography aligned with a PH show
 * region; `review` = region mismatch, non-US, or no PH region to compare —
 * a possible touring act or namesake the admin should VERIFY before linking.
 *
 * `review` is NEVER a gate and is NEVER auto-accepted or hidden: the row is
 * still surfaced and the admin can still accept it. The tier only flags the
 * lower certainty so the reviewer slows down.
 */
export type LinkSuggestionConfidence = 'high' | 'review'

/**
 * One pending suggestion in the review queue, joined to its artist for
 * direct rendering. Mirrors `contracts.LinkSuggestionEntry`. Shape is
 * LOCKED.
 */
export interface LinkSuggestionEntry {
  id: number
  artist_id: number
  artist_name: string
  artist_slug?: string | null
  platform: LinkSuggestionPlatform
  url: string
  source: string
  mb_artist_id?: string | null
  mb_artist_name?: string | null
  confidence: LinkSuggestionConfidence
  region_match: boolean
  live: boolean
  notes?: string | null
  /** Always `pending` in the list response. */
  status: string
  created_at: string
}

/**
 * Paginated review-queue response. Mirrors
 * `contracts.LinkSuggestionListResult`. Shape is LOCKED.
 */
export interface LinkSuggestionListResult {
  suggestions: LinkSuggestionEntry[]
  total: number
}

/**
 * Response from accept/reject. Mirrors
 * `contracts.LinkSuggestionReviewResult`. Shape is LOCKED.
 */
export interface LinkSuggestionReviewResult {
  id: number
  artist_id: number
  /** Resulting status: `accepted` or `rejected`. */
  status: string
  reviewed_at?: string | null
  reviewed_by_user_id?: number | null
}

/**
 * Pagination default. Backend caps `limit` at 200; the UI uses 25 so the
 * queue fits one screen for a typical triage session and keeps server
 * round-trips small.
 */
export const LINK_SUGGESTIONS_DEFAULT_LIMIT = 25

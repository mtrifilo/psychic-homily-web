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
 *
 * The wire shapes below are generated, not hand-written — regenerate with
 * `bun run api:types`.
 */
import type { components } from '@/types/api'

/**
 * Streaming platform a suggestion targets. A DOCUMENTATION type, not a wire
 * type: the OpenAPI document types `platform` as a plain string, so the
 * generated entry carries `string` and this records the value domain.
 */
export type LinkSuggestionPlatform = 'bandcamp' | 'spotify'

/**
 * One pending suggestion in the review queue, joined to its artist for
 * direct rendering. Shape is LOCKED.
 *
 * `confidence` is the region confidence tier (PSY-1191 semantics, carried
 * through the sweep): `high` = the MusicBrainz candidate's geography aligned
 * with a PH show region; `review` = region mismatch, non-US, or no PH region
 * to compare — a possible touring act or namesake the admin should VERIFY
 * before linking. `review` is NEVER a gate and is NEVER auto-accepted or
 * hidden: the row is still surfaced and the admin can still accept it. The
 * tier only flags the lower certainty so the reviewer slows down. The spec
 * types the field as a plain string, so it is not a union here.
 */
export type LinkSuggestionEntry = components['schemas']['LinkSuggestionEntry']

/**
 * Paginated review-queue response. Shape is LOCKED.
 *
 * `suggestions` is nullable on the wire: the Go field is
 * `[]LinkSuggestionEntry` with no `omitempty`, so a nil slice marshals to
 * JSON `null`, not `[]`. Consumers must guard.
 */
export type LinkSuggestionListResult = components['schemas']['LinkSuggestionListResult']

/** Response from accept/reject. Shape is LOCKED. */
export type LinkSuggestionReviewResult = components['schemas']['LinkSuggestionReviewResult']

/**
 * Pagination default. Backend caps `limit` at 200; the UI uses 25 so the
 * queue fits one screen for a typical triage session and keeps server
 * round-trips small.
 */
export const LINK_SUGGESTIONS_DEFAULT_LIMIT = 25

/**
 * Wire-shape types for the admin streaming-discovery worklist endpoints.
 * Mirrors `backend/internal/api/handlers/pipeline/streaming_worklist.go`
 * and `backend/internal/services/contracts/pipeline.go`.
 *
 * The list endpoint surfaces ONLY non-terminal statuses (`unreviewed`,
 * `candidates_pending`). Terminal statuses exist as valid TARGETS for
 * the status-mutation endpoint — see `STATUS_TRANSITION_TARGETS`.
 */
export type StreamingDiscoveryStatus =
  | 'unreviewed'
  | 'candidates_pending'
  | 'linked'
  | 'no_links_found'
  | 'skipped'

/**
 * The two non-terminal statuses surfaced by the list endpoint. An
 * unset filter ("") returns both. Any other value triggers a 400.
 */
export type StreamingWorklistStatusFilter = '' | 'unreviewed' | 'candidates_pending'

export const STREAMING_WORKLIST_STATUS_FILTER_OPTIONS: ReadonlyArray<{
  value: StreamingWorklistStatusFilter
  label: string
}> = [
  { value: '', label: 'All non-terminal' },
  { value: 'unreviewed', label: 'Unreviewed' },
  { value: 'candidates_pending', label: 'Candidates pending' },
] as const

/**
 * Mirror of `contracts.StreamingWorklistEntry`. Soonest event date is an
 * ISO timestamp string after JSON serialisation. Venue name + city are
 * nullable because the LATERAL subquery may surface a show that lacks
 * those fields (rare; usually a data-quality nit).
 */
export interface StreamingWorklistEntry {
  artist_id: number
  artist_name: string
  artist_slug?: string | null
  streaming_discovery_status: StreamingDiscoveryStatus
  soonest_event_date: string
  venue_name?: string | null
  venue_city?: string | null
  upcoming_show_count: number
}

export interface StreamingWorklistResult {
  entries: StreamingWorklistEntry[]
  total: number
}

/**
 * Input shape for the status mutation. `reason` is only persisted for
 * `no_links_found` and `skipped`; the backend service clears it on
 * re-open to `unreviewed`.
 */
export interface UpdateStreamingDiscoveryStatusInput {
  artist_id: number
  status: StreamingDiscoveryStatus
  reason?: string | null
}

/**
 * Backend response after a successful status mutation. Huma's
 * response struct has a `Body` field but the framework serializes that
 * field's value AS the HTTP body — there is no `{body: ...}` envelope
 * on the wire. This type mirrors what the client receives directly.
 */
export interface UpdateStreamingDiscoveryStatusResponseBody {
  id: number
  name: string
  slug?: string | null
  streaming_discovery_status: StreamingDiscoveryStatus
  streaming_discovery_reason?: string | null
  updated_at: string
}

/**
 * Targets surfaced as buttons on each worklist row. Engine seam: the
 * worklist concentrates the state write — `linked` is set by the admin
 * AFTER reviewing the candidate panel on the artist detail page (see
 * engine-seam comment in StreamingWorklist.tsx). The engine itself
 * stays stateless.
 */
export type StreamingWorklistAction = 'linked' | 'no_links_found' | 'skipped'

/**
 * Pagination defaults. Backend caps `limit` at 200; the UI uses 25 so
 * the table fits one screen for typical admin sessions and keeps server
 * round-trips small.
 */
export const STREAMING_WORKLIST_DEFAULT_LIMIT = 25

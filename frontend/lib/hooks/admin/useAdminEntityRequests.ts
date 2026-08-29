'use client'

/**
 * Admin Entity-Request Hooks (PSY-871)
 *
 * TanStack Query hooks for the moderation queue's 4th card type — queued
 * entity-CREATION requests (entity_requests). Mirrors useAdminPendingEdits:
 * one list query + one decide mutation. The decide endpoint approves
 * (→ creates the catalog entity, PSY-1008) or rejects with a note.
 */

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../../api'
import { queryKeys, createInvalidateQueries } from '../../queryClient'

// ─── Types ───────────────────────────────────────────────────────────────────
//
// Aliased from the generated OpenAPI types, not hand-written (PSY-1550/1600).
// Regenerate with `bun run api:types`; the "API Types Drift" CI gate fails if
// the committed types drift from the backend. Exported names are kept stable
// for callers.

import type { components } from '../../../types/api'

/**
 * Optional AI-extraction source context attached to a request (PSY-1008).
 * Stored opaquely server-side; both fields optional.
 */
export type EntityRequestSourceDetail =
  components['schemas']['EntityRequestSourceDetail']

/**
 * Admin-queue view of an entity_requests row. The backend resolves the
 * requester display (the raw model serializes the relation as json:"-") and
 * carries the creation payload for the card's key:value preview.
 *
 * `payload` and `source_detail` are `*json.RawMessage` server-side, which the
 * spec can only describe as `unknown`, so they are narrowed here — the ONLY
 * two fields this module still asserts a shape for. Both are pointers without
 * a non-null guarantee, so both are declared nullable and every read must
 * guard (`req.payload || {}`, `request.source_detail?.url`).
 */
export type AdminEntityRequest = Omit<
  components['schemas']['AdminEntityRequestView'],
  'payload' | 'source_detail'
> & {
  /** Creation payload (shape varies by entity_type); rendered key:value. */
  payload: Record<string, unknown> | null
  source_detail?: EntityRequestSourceDetail | null
}

export type AdminEntityRequestsListResponse = Omit<
  components['schemas']['AdminListEntityRequestsResponseBody'],
  'requests'
> & {
  requests: AdminEntityRequest[] | null
}

// ─── Filters ─────────────────────────────────────────────────────────────────

export interface AdminEntityRequestsFilters {
  state?: string
  entity_type?: string
  source_context?: string
  /**
   * PSY-1088: narrow to approved-but-unfulfilled rows (created_entity_id IS
   * NULL) — the "needs attention" rescue queue. Pair with state='approved'.
   */
  unfulfilled?: boolean
  limit?: number
  offset?: number
  /** When false, the query does not fire (e.g. the admin nav badge off-route). Defaults to true. */
  enabled?: boolean
}

// ─── Hooks ───────────────────────────────────────────────────────────────────

/**
 * Fetch queued entity-creation requests for admin review. Defaults to pending.
 */
export function useAdminEntityRequests(filters: AdminEntityRequestsFilters = {}) {
  const {
    state = 'pending',
    entity_type,
    source_context,
    unfulfilled,
    limit = 50,
    offset = 0,
    enabled = true,
  } = filters

  const params = new URLSearchParams()
  if (state) params.set('state', state)
  if (entity_type) params.set('entity_type', entity_type)
  if (source_context) params.set('source_context', source_context)
  if (unfulfilled) params.set('unfulfilled', 'true')
  params.set('limit', limit.toString())
  params.set('offset', offset.toString())

  const endpoint = `${API_ENDPOINTS.ADMIN.ENTITY_REQUESTS.LIST}?${params.toString()}`

  return useQuery({
    queryKey: queryKeys.admin.entityRequests({
      state,
      entity_type,
      source_context,
      unfulfilled,
      limit,
      offset,
    }),
    queryFn: async (): Promise<AdminEntityRequestsListResponse> => {
      return apiRequest<AdminEntityRequestsListResponse>(endpoint, {
        method: 'GET',
      })
    },
    staleTime: 30 * 1000, // 30 seconds
    enabled,
  })
}

/** Admin-supplied venue for fulfilling a show request (PSY-1037). */
export type ShowVenueInput = components['schemas']['ShowVenueInput']

/**
 * One admin-supplied artist for fulfilling a show request (PSY-1037).
 *
 * Aliased from the generated schema rather than hand-written (PSY-1856): the
 * local interface these two names used to declare SHADOWED the spec, so
 * `bun run typecheck` could not see drift between them — `set_type` existed on
 * the endpoint from PSY-1705 and was invisible here until somebody noticed by
 * eye.
 *
 * `set_type` is optional, and its optionality is load-bearing: only an ABSENT
 * key means "slot unknown". A present value must be in the vocabulary, so
 * callers must OMIT the field rather than send an empty string.
 */
export type ShowArtistInput = components['schemas']['ShowArtistInput']

export interface DecideEntityRequestVars {
  id: number
  decision: 'approved' | 'rejected'
  /** Required by the queue UI when rejecting; omitted on approve. */
  note?: string
  /** PSY-1037: required when approving a show request; ignored otherwise. */
  show_venue?: ShowVenueInput
  show_artists?: ShowArtistInput[]
}

/**
 * Decide a queued entity request. 'approved' creates the catalog entity
 * (PSY-1008) and returns created_entity_id; 'rejected' records the note.
 * Show approvals additionally carry the admin-collected venue + artists
 * (PSY-1037) — the payload alone lacks the associations CreateShow needs.
 * Invalidates the request queue + the entity lists an approval may have grown.
 */
export function useDecideEntityRequest() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async ({ id, decision, note, show_venue, show_artists }: DecideEntityRequestVars) => {
      return apiRequest(API_ENDPOINTS.ADMIN.ENTITY_REQUESTS.DECIDE(id), {
        method: 'POST',
        body: JSON.stringify({
          decision,
          ...(note ? { note } : {}),
          ...(show_venue ? { show_venue } : {}),
          // Lossless on purpose (PSY-1858): an empty array must reach the server
          // as an empty array. The backend reads an ABSENT show_artists as "use
          // the bill on the request payload" and an explicit [] as "the admin
          // removed every act", so collapsing [] into absent would resurrect a
          // bill the admin had just emptied. Both spellings are a 422 today; the
          // difference bites once the form prefills from payload.artists
          // (PSY-1955).
          ...(show_artists !== undefined ? { show_artists } : {}),
        }),
      })
    },
    onSuccess: () => {
      invalidateQueries.adminEntityRequests()
      // Approve creates a catalog entity, so refresh every entity list a
      // fulfillment can grow (one invalidation per fulfillable request type).
      invalidateQueries.artists()
      invalidateQueries.venues()
      invalidateQueries.labels()
      invalidateQueries.releases()
      invalidateQueries.festivals()
      invalidateQueries.shows()
    },
  })
}

/** PSY-1088: rescue action on an approved-but-unfulfilled request. */
export interface RescueEntityRequestVars {
  id: number
  /** 'fulfill' re-runs the catalog create; 'void' rejects the orphan. */
  action: 'fulfill' | 'void'
  /** Recorded as the decision note when voiding. */
  note?: string
  /** Required when fulfilling a SHOW request; ignored otherwise. */
  show_venue?: ShowVenueInput
  show_artists?: ShowArtistInput[]
}

/**
 * Rescue an approved-but-unfulfilled entity request (PSY-1088). 'fulfill'
 * re-runs the catalog create (supplying show associations for a show) and
 * stamps created_entity_id; 'void' rejects the orphan. Bypasses the decide
 * flow (which only re-processes pending rows). Invalidates the request queue +
 * the entity lists a fulfillment may have grown.
 */
export function useRescueEntityRequest() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async ({ id, action, note, show_venue, show_artists }: RescueEntityRequestVars) => {
      return apiRequest(API_ENDPOINTS.ADMIN.ENTITY_REQUESTS.FULFILL(id), {
        method: 'POST',
        body: JSON.stringify({
          action,
          ...(note ? { note } : {}),
          ...(show_venue ? { show_venue } : {}),
          // Lossless on purpose (PSY-1858): an empty array must reach the server
          // as an empty array. The backend reads an ABSENT show_artists as "use
          // the bill on the request payload" and an explicit [] as "the admin
          // removed every act", so collapsing [] into absent would resurrect a
          // bill the admin had just emptied. Both spellings are a 422 today; the
          // difference bites once the form prefills from payload.artists
          // (PSY-1955).
          ...(show_artists !== undefined ? { show_artists } : {}),
        }),
      })
    },
    onSuccess: () => {
      invalidateQueries.adminEntityRequests()
      // Fulfill creates a catalog entity; refresh every list it may have grown.
      invalidateQueries.artists()
      invalidateQueries.venues()
      invalidateQueries.labels()
      invalidateQueries.releases()
      invalidateQueries.festivals()
      invalidateQueries.shows()
    },
  })
}

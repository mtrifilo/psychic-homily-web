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

/**
 * Optional AI-extraction source context attached to a request (PSY-1008).
 * Stored opaquely server-side; both fields optional.
 */
export interface EntityRequestSourceDetail {
  url?: string
  excerpt?: string
}

/**
 * Admin-queue view of an entity_requests row. The backend resolves the
 * requester display (the raw model serializes the relation as json:"-") and
 * carries the typed creation payload for the card's key:value preview.
 */
export interface AdminEntityRequest {
  id: number
  entity_type: string
  /** Typed creation payload (shape varies by entity_type); rendered key:value. */
  payload: Record<string, unknown>
  source_context: string
  source_detail?: EntityRequestSourceDetail | null
  requester_id: number
  requester_name: string
  /**
   * Requester's username when set, null otherwise. Pass to
   * `<UserAttribution username={...} />` to link the byline to /users/:username.
   */
  requester_username: string | null
  decision_state: 'pending' | 'approved' | 'rejected'
  decision_note?: string | null
  /** The catalog entity created when the request was fulfilled (PSY-1008). */
  created_entity_id?: number | null
  created_at: string
}

export interface AdminEntityRequestsListResponse {
  requests: AdminEntityRequest[]
  total: number
}

// ─── Filters ─────────────────────────────────────────────────────────────────

export interface AdminEntityRequestsFilters {
  state?: string
  entity_type?: string
  source_context?: string
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
    limit = 50,
    offset = 0,
    enabled = true,
  } = filters

  const params = new URLSearchParams()
  if (state) params.set('state', state)
  if (entity_type) params.set('entity_type', entity_type)
  if (source_context) params.set('source_context', source_context)
  params.set('limit', limit.toString())
  params.set('offset', offset.toString())

  const endpoint = `${API_ENDPOINTS.ADMIN.ENTITY_REQUESTS.LIST}?${params.toString()}`

  return useQuery({
    queryKey: queryKeys.admin.entityRequests({
      state,
      entity_type,
      source_context,
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
export interface ShowVenueInput {
  name: string
  city: string
  state: string
  address?: string
}

/** One admin-supplied artist for fulfilling a show request (PSY-1037). */
export interface ShowArtistInput {
  name: string
  is_headliner?: boolean
}

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
          ...(show_artists?.length ? { show_artists } : {}),
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

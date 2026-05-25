'use client'

/**
 * TanStack Query hooks for the admin streaming-discovery worklist.
 *
 * Two hooks mirror the two backend endpoints:
 *   - useStreamingWorklist — GET /admin/streaming-worklist
 *   - useUpdateStreamingDiscoveryStatus — POST /admin/artists/{id}/streaming-discovery-status
 *
 * Mutations invalidate the worklist list query so the row drops out
 * of the visible queue once the new status flips to a terminal value
 * (or to candidates_pending, which is engine-set and not surfaced as a
 * mutation target here). Errors bubble up unchanged so the consuming
 * component can surface them via the canonical inline-banner primitive
 * (pattern_mutation_feedback.md).
 */

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
import type {
  StreamingWorklistResult,
  StreamingWorklistStatusFilter,
  UpdateStreamingDiscoveryStatusInput,
  UpdateStreamingDiscoveryStatusResponseBody,
} from './types'

export interface UseStreamingWorklistParams {
  status: StreamingWorklistStatusFilter
  limit: number
  offset: number
  enabled?: boolean
}

/**
 * Fetch a paginated page of the streaming-discovery worklist. Backend
 * surfaces ONLY non-terminal statuses; passing a status filter narrows
 * to one of `unreviewed` or `candidates_pending`. Empty filter returns
 * both. `staleTime` is short (10s) so the list reflects status
 * mutations promptly — windowed-focus refetches handle the case where
 * the admin reviewed a candidate on the artist detail page and the
 * worklist tab regains focus.
 */
export function useStreamingWorklist({
  status,
  limit,
  offset,
  enabled = true,
}: UseStreamingWorklistParams) {
  return useQuery({
    queryKey: queryKeys.admin.streamingWorklist({ status, limit, offset }),
    queryFn: () => {
      const search = new URLSearchParams()
      search.set('limit', String(limit))
      search.set('offset', String(offset))
      if (status) {
        search.set('status', status)
      }
      return apiRequest<StreamingWorklistResult>(
        `${API_ENDPOINTS.ADMIN.STREAMING_WORKLIST.LIST}?${search.toString()}`
      )
    },
    enabled,
    staleTime: 10 * 1000,
    // Refetch on window focus so the worklist updates when the admin
    // hops back from the artist detail page after reviewing candidates.
    refetchOnWindowFocus: true,
  })
}

/**
 * Mutate a row's streaming-discovery status. Used by the action column
 * for `linked` / `no_links_found` / `skipped`. The engine-seam choice
 * (worklist concentrates the state write) means this is also the hook
 * the page calls AFTER the admin reviewed candidates on the artist
 * detail page and confirmed the URLs got saved.
 *
 * Invalidates the full `streamingWorklist` branch so every cached page
 * (status filter / pagination combination) refetches.
 */
export function useUpdateStreamingDiscoveryStatus() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (
      input: UpdateStreamingDiscoveryStatusInput
    ): Promise<UpdateStreamingDiscoveryStatusResponseBody> =>
      // Huma's response struct has a `Body` field, but the framework
      // serializes that field's value AS the HTTP body — there is no
      // `{body: ...}` envelope on the wire. The response IS the artist
      // payload. Verified empirically against the dev backend.
      apiRequest<UpdateStreamingDiscoveryStatusResponseBody>(
        API_ENDPOINTS.ADMIN.ARTISTS.STREAMING_DISCOVERY_STATUS(input.artist_id),
        {
          method: 'POST',
          body: JSON.stringify({
            status: input.status,
            reason: input.reason ?? null,
          }),
        }
      ),
    onSuccess: () => {
      // Invalidate the whole worklist branch — status filter and
      // pagination both vary by key, so a single invalidation needs to
      // hit every cached page.
      queryClient.invalidateQueries({
        queryKey: ['admin', 'streamingWorklist'],
      })
    },
  })
}

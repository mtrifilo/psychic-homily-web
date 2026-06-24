'use client'

/**
 * TanStack Query hooks for the admin music-link suggestion review queue
 * (PSY-1207).
 *
 * Two hooks mirror the three backend endpoints (accept/reject share one
 * mutation hook, parameterised by verdict):
 *   - useLinkSuggestions    — GET  /admin/link-suggestions
 *   - useReviewLinkSuggestion — POST /admin/link-suggestions/{id}/{accept|reject}
 *
 * The review mutation invalidates the whole linkSuggestions branch so every
 * cached page (pagination combination) refetches and the reviewed row drops
 * out of the visible queue. An accept also invalidates the artists prefix:
 * accepting writes the artist's Bandcamp/Spotify link server-side, so the
 * artist detail page must refetch to render the new embed (Spotify is
 * immediate; Bandcamp fills shortly after the PSY-1190 resolver runs async).
 *
 * Errors bubble up unchanged (apiRequest throws an ApiError carrying
 * `.status`) so the consuming component can surface 404/409/422 distinctly
 * instead of silently dropping the row.
 */

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
import type { LinkSuggestionListResult, LinkSuggestionReviewResult } from './types'

export interface UseLinkSuggestionsParams {
  limit: number
  offset: number
  enabled?: boolean
}

/**
 * Fetch a paginated page of pending link suggestions. The backend returns
 * ONLY `pending` rows, high-confidence first. `staleTime` is short (10s) so
 * the list reflects accept/reject mutations promptly.
 */
export function useLinkSuggestions({
  limit,
  offset,
  enabled = true,
}: UseLinkSuggestionsParams) {
  return useQuery({
    queryKey: queryKeys.admin.linkSuggestions({ limit, offset }),
    queryFn: () => {
      const search = new URLSearchParams()
      search.set('limit', String(limit))
      search.set('offset', String(offset))
      return apiRequest<LinkSuggestionListResult>(
        `${API_ENDPOINTS.ADMIN.LINK_SUGGESTIONS.LIST}?${search.toString()}`
      )
    },
    enabled,
    staleTime: 10 * 1000,
  })
}

/** Accept writes the link; reject just marks the row. */
export type LinkSuggestionVerdict = 'accept' | 'reject'

export interface ReviewLinkSuggestionInput {
  suggestionId: number
  verdict: LinkSuggestionVerdict
}

/**
 * Accept or reject a pending suggestion. Both endpoints take NO body — the
 * verdict is in the path (`/accept` vs `/reject`). Accept writes the link
 * via the existing artist update path and marks the row accepted; reject
 * marks it rejected. Both stamp the reviewer and are idempotent on replay;
 * a re-review with a conflicting verdict returns a 409.
 *
 * Invalidates the whole linkSuggestions branch (so the reviewed row drops
 * out of every cached page) and, on an accept, the artists prefix (so the
 * artist detail page refetches and renders the freshly-written embed).
 */
export function useReviewLinkSuggestion() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({
      suggestionId,
      verdict,
    }: ReviewLinkSuggestionInput): Promise<LinkSuggestionReviewResult> => {
      const url =
        verdict === 'accept'
          ? API_ENDPOINTS.ADMIN.LINK_SUGGESTIONS.ACCEPT(suggestionId)
          : API_ENDPOINTS.ADMIN.LINK_SUGGESTIONS.REJECT(suggestionId)
      // Huma serializes the response struct's `Body` field AS the HTTP body —
      // there is no `{body: ...}` envelope on the wire. apiRequest returns the
      // review result directly.
      return apiRequest<LinkSuggestionReviewResult>(url, { method: 'POST' })
    },
    onSuccess: (_result, variables) => {
      // Drop the reviewed row from every cached page (pagination varies by
      // key, so a single invalidation needs to hit the whole branch).
      queryClient.invalidateQueries({
        queryKey: ['admin', 'linkSuggestions'],
      })
      // An accept writes the artist's social link server-side; refetch the
      // artists prefix so the detail page renders the new embed. (Reject
      // touches nothing on the artist, so skip the broad invalidation there.)
      if (variables.verdict === 'accept') {
        queryClient.invalidateQueries({ queryKey: queryKeys.artists.all })
      }
    },
  })
}

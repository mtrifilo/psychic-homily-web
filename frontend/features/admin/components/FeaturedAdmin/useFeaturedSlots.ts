'use client'

/**
 * TanStack Query hooks for the admin featured-slot endpoints.
 *
 * Three hooks mirror the three backend endpoints (PSY-835):
 *   - useFeaturedSlots — GET /admin/featured-slots (both types in one call)
 *   - useSetFeaturedSlot — POST /admin/featured-slots (atomic retire + insert)
 *   - useRetireFeaturedSlot — DELETE /admin/featured-slots/{slot_type}
 *
 * Mutations invalidate the list query so both panels re-render on the
 * next paint. Errors bubble up unchanged so the consuming component can
 * surface them via the canonical inline-banner primitive
 * (pattern_mutation_feedback.md).
 */

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
import type {
  ExploreFeaturedResponse,
  FeaturedSlotType,
  ListFeaturedSlotsResponse,
  RetireFeaturedSlotResponse,
  SetFeaturedSlotInput,
  SetFeaturedSlotResponse,
} from './types'

/**
 * Fetch both the Featured Bill + Featured Collection state in one
 * request. Backend returns one entry per slot type with the active
 * row (or null) and recent history. Short staleTime (15s) so the
 * panel reflects mutations quickly without polling.
 */
export function useFeaturedSlots(options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: queryKeys.admin.featuredSlots(),
    queryFn: () =>
      apiRequest<ListFeaturedSlotsResponse>(
        API_ENDPOINTS.ADMIN.FEATURED_SLOTS.LIST
      ),
    enabled: options?.enabled ?? true,
    staleTime: 15 * 1000,
  })
}

/**
 * Fetch the hydrated active referents (bill + collection) from the
 * public /explore/featured endpoint. Used by the admin page as the
 * read source for the "current active" slot cards — the admin
 * endpoint returns only entity_id, so we delegate name/thumbnail
 * resolution to the existing public endpoint. Both fields are
 * nullable when nothing is set or the referent is no longer
 * publicly visible. Same invalidation key as the admin list so a
 * Set / Retire mutation refreshes both queries.
 */
export function useExploreFeatured(options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: queryKeys.explore.featured,
    queryFn: () =>
      apiRequest<ExploreFeaturedResponse>(API_ENDPOINTS.EXPLORE.FEATURED),
    enabled: options?.enabled ?? true,
    staleTime: 15 * 1000,
  })
}

/**
 * Set a new active pick for a slot type. Backend atomically retires
 * the previous active row (`active_until = NOW()`) and inserts the new
 * one inside one transaction, so the unique-active-row invariant
 * always holds. Invalidates the list query so both panels refresh.
 */
export function useSetFeaturedSlot() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: SetFeaturedSlotInput) =>
      apiRequest<SetFeaturedSlotResponse>(
        API_ENDPOINTS.ADMIN.FEATURED_SLOTS.SET,
        {
          method: 'POST',
          body: JSON.stringify(input),
        }
      ),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.admin.featuredSlots(),
      })
      // Also bust the public /explore/featured cache so the active
      // card on the admin page (which reads from this endpoint for
      // hydrated referent details) reflects the change immediately.
      queryClient.invalidateQueries({
        queryKey: queryKeys.explore.featured,
      })
    },
  })
}

/**
 * Retire the current active row for a slot type without replacement.
 * Idempotent at the API surface — a second DELETE returns 404, which
 * the consumer can treat as a no-op for UI purposes. The /explore
 * Featured section collapses for that slot on next render.
 */
export function useRetireFeaturedSlot() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (slotType: FeaturedSlotType) =>
      apiRequest<RetireFeaturedSlotResponse>(
        API_ENDPOINTS.ADMIN.FEATURED_SLOTS.RETIRE(slotType),
        {
          method: 'DELETE',
        }
      ),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.admin.featuredSlots(),
      })
      // Also bust the public /explore/featured cache so the active
      // card on the admin page (which reads from this endpoint for
      // hydrated referent details) reflects the change immediately.
      queryClient.invalidateQueries({
        queryKey: queryKeys.explore.featured,
      })
    },
  })
}

'use client'

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { createInvalidateQueries } from '@/lib/queryClient'
import { showEndpoints } from '@/features/shows/api'
import { showLogger } from '@/lib/utils/showLogger'
import { ShowError } from '@/lib/errors'
import type { SetType, ShowResponse, OrphanedArtist } from '../types'

/**
 * Venue data for show update requests
 * Either id (for existing venue) or name (for new/lookup) should be provided
 */
export interface ShowUpdateVenue {
  id?: number
  name?: string
  city?: string
  state?: string
  address?: string
}

/**
 * Artist data for show update requests
 * Either id (for existing artist) or name (for new/lookup) should be provided
 */
export interface ShowUpdateArtist {
  id?: number
  name?: string
  /**
   * Legacy headliner flag, ignored when set_type is present. Leaving BOTH this
   * and set_type undefined hands the slot to the server's bill-level rule: the
   * act is stored as the headliner only when it is first on a bill where no act
   * names one. Once any act names a headliner, the silent acts store
   * 'performer'. Send both fields per act (as toArtistPayloads does) to state
   * the bill outright and keep the update independent of list order.
   */
  is_headliner?: boolean
  /**
   * Curated bill role. Authoritative on the server when present: is_headliner
   * is derived from it. Omit to leave the slot uncurated, subject to the
   * bill-level rule described on is_headliner.
   */
  set_type?: SetType
  instagram_handle?: string
}

/**
 * Show update request payload
 * Matches the backend UpdateShowRequest body
 * All fields are optional for partial updates
 *
 * When venues or artists arrays are provided, they replace the existing
 * show associations entirely. Omit them to keep existing associations.
 */
export interface ShowUpdate {
  title?: string
  event_date?: string // ISO 8601 UTC timestamp
  city?: string
  state?: string
  // Advance price and door price. An omitted field leaves the stored value
  // alone, so writing one never clears the other.
  price?: number
  door_price?: number
  age_requirement?: string
  description?: string
  image_url?: string
  venues?: ShowUpdateVenue[]
  artists?: ShowUpdateArtist[]
}

/**
 * Extended show response with optional error fields and orphaned artists
 */
export interface ShowUpdateResponse extends ShowResponse {
  error_code?: string
  request_id?: string
  orphaned_artists?: OrphanedArtist[]
}

/**
 * Hook for updating an existing show
 * Requires authentication (JWT cookie handled by API proxy)
 */
export function useShowUpdate() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async ({
      showId,
      updates,
    }: {
      showId: number
      updates: ShowUpdate
    }): Promise<ShowUpdateResponse> => {
      const updateFields = Object.keys(updates).filter(
        key => updates[key as keyof ShowUpdate] !== undefined
      )

      showLogger.updateAttempt(showId, updateFields)

      const payload = JSON.stringify(updates)
      const response = await apiRequest<ShowUpdateResponse>(
        showEndpoints.UPDATE(showId),
        {
          method: 'PUT',
          body: payload,
        }
      )

      return response
    },
    onSuccess: (data, variables) => {
      showLogger.updateSuccess(
        variables.showId,
        (data as ShowUpdateResponse).request_id
      )

      // Invalidate show queries to refetch with updated data
      invalidateQueries.shows()
      // Also invalidate artists and venues in case they were modified
      invalidateQueries.artists()
      invalidateQueries.venues()
    },
    onError: (error, variables) => {
      const showError = ShowError.fromUnknown(error, variables.showId)
      showLogger.updateFailed(
        variables.showId,
        showError.code,
        showError.message,
        showError.requestId
      )
    },
  })
}

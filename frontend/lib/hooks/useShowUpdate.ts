'use client'

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../api'
import { createInvalidateQueries } from '../queryClient'
import { showLogger } from '../utils/showLogger'
import { ShowError } from '../errors'
import type { ShowResponse, OrphanedArtist } from '../types/show'

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
  is_headliner?: boolean
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
  price?: number
  age_requirement?: string
  description?: string
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
        API_ENDPOINTS.SHOWS.UPDATE(showId),
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

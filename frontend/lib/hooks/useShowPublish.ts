'use client'

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../api'
import { createInvalidateQueries } from '../queryClient'
import { showLogger } from '../utils/showLogger'
import { ShowError } from '../errors'
import type { ShowResponse } from '../types/show'

/**
 * Hook for publishing a private show
 * If all venues are verified, status becomes approved.
 * If any venue is unverified, status becomes pending.
 * Requires authentication (JWT cookie handled by API proxy)
 * User must be admin or the show's submitter
 */
export function useShowPublish() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async (showId: number): Promise<ShowResponse> => {
      showLogger.debug('Publishing show', { showId })

      return await apiRequest<ShowResponse>(
        API_ENDPOINTS.SHOWS.PUBLISH(showId),
        {
          method: 'POST',
        }
      )
    },
    onSuccess: (data, showId) => {
      showLogger.debug('Show published', { showId, status: data.status })

      // Invalidate show queries to refetch with updated data
      invalidateQueries.shows()
      // Also invalidate saved shows to update status display
      invalidateQueries.savedShows()
    },
    onError: (error, showId) => {
      const showError = ShowError.fromUnknown(error, showId)
      showLogger.error('Failed to publish show', {
        showId,
        code: showError.code,
        message: showError.message,
        requestId: showError.requestId,
      })
    },
  })
}

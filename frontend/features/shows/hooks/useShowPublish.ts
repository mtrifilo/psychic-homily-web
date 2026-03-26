'use client'

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { createInvalidateQueries } from '@/lib/queryClient'
import { showEndpoints } from '@/features/shows/api'
import { showLogger } from '@/lib/utils/showLogger'
import { ShowError } from '@/lib/errors'
import type { ShowResponse } from '../types'

/**
 * Hook for publishing a private show
 * Shows are always approved regardless of venue verification status.
 * Unverified venues will display city-only until verified by an admin.
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
        showEndpoints.PUBLISH(showId),
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

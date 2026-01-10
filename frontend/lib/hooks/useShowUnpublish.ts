'use client'

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../api'
import { createInvalidateQueries } from '../queryClient'
import { showLogger } from '../utils/showLogger'
import { ShowError } from '../errors'
import type { ShowResponse } from '../types/show'

/**
 * Hook for unpublishing a show (changing status from approved to pending)
 * Requires authentication (JWT cookie handled by API proxy)
 * User must be admin or the show's submitter
 */
export function useShowUnpublish() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async (showId: number): Promise<ShowResponse> => {
      showLogger.unpublishAttempt(showId)

      return await apiRequest<ShowResponse>(
        API_ENDPOINTS.SHOWS.UNPUBLISH(showId),
        {
          method: 'POST',
        }
      )
    },
    onSuccess: (data, showId) => {
      showLogger.unpublishSuccess(showId)

      // Invalidate show queries to refetch with updated data
      invalidateQueries.shows()
      // Also invalidate saved shows to update status display
      invalidateQueries.savedShows()
    },
    onError: (error, showId) => {
      const showError = ShowError.fromUnknown(error, showId)
      showLogger.unpublishFailed(
        showId,
        showError.code,
        showError.message,
        showError.requestId
      )
    },
  })
}

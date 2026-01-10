'use client'

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../api'
import { createInvalidateQueries } from '../queryClient'
import { showLogger } from '../utils/showLogger'
import { ShowError } from '../errors'

/**
 * Hook for deleting a show
 * Requires authentication (JWT cookie handled by API proxy)
 * User must be admin or the show's submitter
 */
export function useShowDelete() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async (showId: number): Promise<void> => {
      showLogger.deleteAttempt(showId)

      await apiRequest(API_ENDPOINTS.SHOWS.DELETE(showId), {
        method: 'DELETE',
      })
    },
    onSuccess: (_, showId) => {
      showLogger.deleteSuccess(showId)

      // Invalidate show queries to refetch with updated data
      invalidateQueries.shows()
      // Also invalidate saved shows in case the deleted show was saved
      invalidateQueries.savedShows()
    },
    onError: (error, showId) => {
      const showError = ShowError.fromUnknown(error, showId)
      showLogger.deleteFailed(
        showId,
        showError.code,
        showError.message,
        showError.requestId
      )
    },
  })
}

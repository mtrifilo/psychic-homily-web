'use client'

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../api'
import { createInvalidateQueries } from '../queryClient'
import { showLogger } from '../utils/showLogger'
import { ShowError, ShowErrorCode } from '../errors'
import type { ShowResponse } from '../types/show'

/**
 * Artist data for show submission
 */
interface SubmitArtist {
  name: string
  id?: number
  is_headliner?: boolean
  instagram_handle?: string
}

/**
 * Venue data for show submission
 */
interface SubmitVenue {
  name: string
  id?: number
  city: string
  state: string
  address?: string
}

/**
 * Show submission request payload
 * Matches the backend CreateShowRequestBody
 */
export interface ShowSubmission {
  title?: string
  event_date: string // ISO 8601 UTC timestamp
  city: string
  state: string
  price?: number
  age_requirement?: string
  description?: string
  venues: SubmitVenue[]
  artists: SubmitArtist[]
  is_private?: boolean // If true, show is private and only visible to submitter
}

/**
 * Extended show response with optional error fields
 */
interface ShowSubmitResponse extends ShowResponse {
  error_code?: string
  request_id?: string
}

/**
 * Hook for submitting a new show
 * Requires authentication (JWT cookie handled by API proxy)
 */
export function useShowSubmit() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async (submission: ShowSubmission): Promise<ShowResponse> => {
      showLogger.submitAttempt({
        venueCount: submission.venues.length,
        artistCount: submission.artists.length,
        city: submission.city,
        state: submission.state,
      })

      const response = await apiRequest<ShowSubmitResponse>(
        API_ENDPOINTS.SHOWS.SUBMIT,
        {
          method: 'POST',
          body: JSON.stringify(submission),
        }
      )

      return response
    },
    onSuccess: (data, variables) => {
      showLogger.submitSuccess(data.id, (data as ShowSubmitResponse).request_id)

      // Invalidate show queries to refetch with new data
      invalidateQueries.shows()
      // Also invalidate artists in case new artists were created
      invalidateQueries.artists()
      // Invalidate saved shows since backend auto-saves to user's list
      invalidateQueries.savedShows()
    },
    onError: (error, variables) => {
      const showError = ShowError.fromUnknown(error)
      showLogger.submitFailed(
        showError.code,
        showError.message,
        showError.requestId
      )
    },
  })
}

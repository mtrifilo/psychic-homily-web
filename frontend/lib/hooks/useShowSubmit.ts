'use client'

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../api'
import { createInvalidateQueries } from '../queryClient'
import type { ShowResponse } from '../types/show'

/**
 * Artist data for show submission
 */
interface SubmitArtist {
  name: string
  id?: number
  is_headliner?: boolean
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
      return apiRequest<ShowResponse>(API_ENDPOINTS.SHOWS.SUBMIT, {
        method: 'POST',
        body: JSON.stringify(submission),
      })
    },
    onSuccess: () => {
      // Invalidate show queries to refetch with new data
      invalidateQueries.shows()
      // Also invalidate artists in case new artists were created
      invalidateQueries.artists()
    },
    onError: error => {
      console.error('Error submitting show:', error)
    },
  })
}


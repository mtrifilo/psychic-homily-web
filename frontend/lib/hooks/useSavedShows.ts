'use client'

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../api'
import { queryKeys, createInvalidateQueries } from '../queryClient'
import type {
  SavedShowsListResponse,
  SaveShowResponse,
  CheckSavedResponse,
} from '../types/show'

interface UseSavedShowsOptions {
  limit?: number
  offset?: number
}

/**
 * Hook to fetch user's saved shows
 * Requires authentication
 */
export const useSavedShows = (options: UseSavedShowsOptions = {}) => {
  const { limit = 50, offset = 0 } = options

  const params = new URLSearchParams()
  params.set('limit', limit.toString())
  params.set('offset', offset.toString())

  const endpoint = `${API_ENDPOINTS.SAVED_SHOWS.LIST}?${params.toString()}`

  return useQuery({
    queryKey: queryKeys.savedShows.list(),
    queryFn: async (): Promise<SavedShowsListResponse> => {
      return apiRequest<SavedShowsListResponse>(endpoint, {
        method: 'GET',
      })
    },
    staleTime: 5 * 60 * 1000, // 5 minutes
  })
}

/**
 * Hook to check if a specific show is saved
 * Requires authentication
 */
export const useIsShowSaved = (showId: number | string | null) => {
  return useQuery({
    queryKey: queryKeys.savedShows.check(String(showId)),
    queryFn: async (): Promise<CheckSavedResponse> => {
      return apiRequest<CheckSavedResponse>(
        API_ENDPOINTS.SAVED_SHOWS.CHECK(showId!),
        {
          method: 'GET',
        }
      )
    },
    enabled: Boolean(showId),
    staleTime: 30 * 1000, // 30 seconds (shorter since save state can change)
  })
}

/**
 * Hook to save a show to user's list
 * Requires authentication
 */
export const useSaveShow = () => {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async (showId: number): Promise<SaveShowResponse> => {
      return apiRequest<SaveShowResponse>(
        API_ENDPOINTS.SAVED_SHOWS.SAVE(showId),
        {
          method: 'POST',
        }
      )
    },
    onSuccess: (_data, showId) => {
      // Invalidate saved shows list
      invalidateQueries.savedShows()
      // Invalidate the specific show's saved status
      queryClient.invalidateQueries({
        queryKey: queryKeys.savedShows.check(String(showId)),
      })
    },
  })
}

/**
 * Hook to unsave (remove) a show from user's list
 * Requires authentication
 */
export const useUnsaveShow = () => {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async (showId: number): Promise<SaveShowResponse> => {
      return apiRequest<SaveShowResponse>(
        API_ENDPOINTS.SAVED_SHOWS.UNSAVE(showId),
        {
          method: 'DELETE',
        }
      )
    },
    onSuccess: (_data, showId) => {
      // Invalidate saved shows list
      invalidateQueries.savedShows()
      // Invalidate the specific show's saved status
      queryClient.invalidateQueries({
        queryKey: queryKeys.savedShows.check(String(showId)),
      })
    },
  })
}

/**
 * Combined hook that provides save/unsave toggle functionality
 * Includes optimistic updates for better UX
 */
export const useSaveShowToggle = (showId: number) => {
  const queryClient = useQueryClient()
  const { data: savedStatus } = useIsShowSaved(showId)
  const saveShow = useSaveShow()
  const unsaveShow = useUnsaveShow()

  const isSaved = savedStatus?.is_saved ?? false
  const isLoading = saveShow.isPending || unsaveShow.isPending

  const toggle = async () => {
    // Optimistic update
    const checkQueryKey = queryKeys.savedShows.check(String(showId))

    queryClient.setQueryData(checkQueryKey, {
      is_saved: !isSaved,
    })

    try {
      if (isSaved) {
        await unsaveShow.mutateAsync(showId)
      } else {
        await saveShow.mutateAsync(showId)
      }
    } catch (error) {
      // Rollback on error
      queryClient.setQueryData(checkQueryKey, {
        is_saved: isSaved,
      })
      throw error
    }
  }

  return {
    isSaved,
    isLoading,
    toggle,
    error: saveShow.error || unsaveShow.error,
  }
}

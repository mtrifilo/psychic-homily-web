'use client'

import { useMemo } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../api'
import { queryKeys, createInvalidateQueries } from '../queryClient'
import type {
  SavedShowsListResponse,
  SaveShowResponse,
} from '../types/show'

interface UseSavedShowsOptions {
  limit?: number
  offset?: number
  enabled?: boolean
}

/**
 * Hook to fetch user's saved shows
 * Requires authentication
 */
export const useSavedShows = (options: UseSavedShowsOptions = {}) => {
  const { limit = 50, offset = 0, enabled = true } = options

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
    enabled,
    staleTime: 5 * 60 * 1000, // 5 minutes
  })
}

/**
 * Hook that returns a Set of saved show IDs from the saved shows list.
 * Fetches the list once instead of making per-show check requests.
 */
export const useSavedShowIds = (isAuthenticated: boolean) => {
  const { data, isLoading } = useSavedShows({ enabled: isAuthenticated })

  const savedIds = useMemo(() => {
    if (!data?.shows) return new Set<number>()
    return new Set(data.shows.map(s => s.id))
  }, [data?.shows])

  return { savedIds, isLoading }
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
    onSuccess: () => {
      invalidateQueries.savedShows()
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
    onSuccess: () => {
      invalidateQueries.savedShows()
    },
  })
}

/**
 * Combined hook that provides save/unsave toggle functionality
 * Uses the saved show IDs set instead of per-show API requests.
 * Includes optimistic updates for better UX.
 */
export const useSaveShowToggle = (showId: number, isAuthenticated: boolean) => {
  const queryClient = useQueryClient()
  const { savedIds } = useSavedShowIds(isAuthenticated)
  const saveShow = useSaveShow()
  const unsaveShow = useUnsaveShow()

  const isSaved = savedIds.has(showId)
  const isLoading = saveShow.isPending || unsaveShow.isPending

  const toggle = async () => {
    // Optimistic update on the saved shows list
    const listQueryKey = queryKeys.savedShows.list()
    const previousData = queryClient.getQueryData<SavedShowsListResponse>(listQueryKey)

    if (previousData) {
      if (isSaved) {
        queryClient.setQueryData(listQueryKey, {
          ...previousData,
          shows: previousData.shows.filter(s => s.id !== showId),
          total: previousData.total - 1,
        })
      } else {
        // For save, we don't have the full show data to add to the list,
        // so just invalidate after mutation succeeds
      }
    }

    try {
      if (isSaved) {
        await unsaveShow.mutateAsync(showId)
      } else {
        await saveShow.mutateAsync(showId)
      }
    } catch (error) {
      // Rollback on error
      if (previousData) {
        queryClient.setQueryData(listQueryKey, previousData)
      }
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

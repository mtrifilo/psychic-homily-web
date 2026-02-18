'use client'

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../api'
import { queryKeys, createInvalidateQueries } from '../queryClient'
import type {
  SavedShowsListResponse,
  SaveShowResponse,
  CheckSavedResponse,
  CheckBatchSavedResponse,
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
 * Hook to batch-check which shows are saved by the current user.
 * Replaces N individual useIsShowSaved calls with a single POST request.
 */
export const useSavedShowBatch = (showIds: number[], enabled: boolean) => {
  return useQuery({
    queryKey: queryKeys.savedShows.batch(showIds),
    queryFn: async (): Promise<CheckBatchSavedResponse> => {
      return apiRequest<CheckBatchSavedResponse>(
        API_ENDPOINTS.SAVED_SHOWS.CHECK_BATCH,
        {
          method: 'POST',
          body: JSON.stringify({ show_ids: showIds }),
        }
      )
    },
    select: (data) => new Set(data.saved_show_ids),
    enabled: showIds.length > 0 && enabled,
    staleTime: 5 * 60 * 1000,
  })
}

/**
 * Hook to check if a specific show is saved
 * Requires authentication
 */
export const useIsShowSaved = (showId: number | string | null, isAuthenticated: boolean, enabled: boolean = true) => {
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
    enabled: Boolean(showId) && isAuthenticated && enabled,
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
export const useSaveShowToggle = (showId: number, isAuthenticated: boolean, batchIsSaved?: boolean) => {
  const queryClient = useQueryClient()
  const { data: savedStatus } = useIsShowSaved(showId, isAuthenticated, batchIsSaved === undefined)
  const saveShow = useSaveShow()
  const unsaveShow = useUnsaveShow()

  const isSaved = batchIsSaved ?? savedStatus?.is_saved ?? false
  const isLoading = saveShow.isPending || unsaveShow.isPending

  const toggle = async () => {
    const checkQueryKey = queryKeys.savedShows.check(String(showId))

    // Cancel any in-flight check queries so stale responses don't overwrite the optimistic update
    await queryClient.cancelQueries({ queryKey: checkQueryKey })

    // Optimistic update
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

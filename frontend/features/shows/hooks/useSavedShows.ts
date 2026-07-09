'use client'

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys, createInvalidateQueries } from '@/lib/queryClient'
// Note: useSavedShows uses SAVED_SHOWS endpoints from lib/api (not show-specific)
import type {
  SavedShowsListResponse,
  SaveShowResponse,
  ShowSaveCount,
  SaveCountEntry,
  BatchSaveCountsResponse,
} from '../types'

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
 * Hook to fetch a single show's public save count (plus the caller's own
 * is_saved when authenticated). Uses optional auth, so it works logged-out.
 */
export const useShowSaveCount = (
  showId: number,
  isAuthenticated: boolean,
  enabled: boolean = true
) => {
  return useQuery({
    queryKey: queryKeys.savedShows.count(showId, isAuthenticated),
    queryFn: async (): Promise<ShowSaveCount> => {
      return apiRequest<ShowSaveCount>(API_ENDPOINTS.SAVE_COUNTS.SHOW(showId), {
        method: 'GET',
      })
    },
    enabled: showId > 0 && enabled,
    staleTime: 2 * 60 * 1000,
  })
}

/**
 * Hook to fetch save counts for many shows in one request.
 *
 * Uses optional auth, so it serves anonymous visitors (counts only) and
 * authenticated ones (counts + is_saved) from the same endpoint. This single
 * call replaces the two the shows list used to fire — one for public counts,
 * one for the viewer's own saved state.
 */
export const useShowSaveCountBatch = (
  showIds: number[],
  isAuthenticated: boolean
) => {
  return useQuery({
    queryKey: queryKeys.savedShows.countBatch(showIds, isAuthenticated),
    queryFn: async (): Promise<Record<string, SaveCountEntry>> => {
      const response = await apiRequest<BatchSaveCountsResponse>(
        API_ENDPOINTS.SAVE_COUNTS.BATCH,
        {
          method: 'POST',
          body: JSON.stringify({ show_ids: showIds }),
        }
      )
      return response.saves
    },
    enabled: showIds.length > 0,
    staleTime: 2 * 60 * 1000,
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
    onSuccess: () => {
      // Re-sync the user's list and every cached save count from the server.
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
      // Re-sync the user's list and every cached save count from the server.
      invalidateQueries.savedShows()
    },
  })
}

/**
 * Save/unsave toggle with optimistic updates.
 *
 * `isSaved` is supplied by the caller rather than fetched here: every caller
 * already holds it, from either the batch or the single save-count query, both
 * of which return is_saved alongside the public count. Re-querying it would
 * mean two requests for the same fact.
 */
export const useSaveShowToggle = (showId: number, isSaved: boolean) => {
  const queryClient = useQueryClient()
  const saveShow = useSaveShow()
  const unsaveShow = useUnsaveShow()

  const isLoading = saveShow.isPending || unsaveShow.isPending

  const toggle = async () => {
    // Toggling requires auth, so the authenticated variant of the key is the
    // only one that can be live for this user.
    const countQueryKey = queryKeys.savedShows.count(showId, true)
    // Prefix filter: patches every cached batch, regardless of its show-id set
    // or auth flag, so a row's count moves the instant the heart is clicked.
    const countBatchPrefix = ['savedShows', 'countBatch']
    const delta = isSaved ? -1 : 1

    // Cancel in-flight reads so stale responses don't overwrite the optimistic update
    await Promise.all([
      queryClient.cancelQueries({ queryKey: countQueryKey }),
      queryClient.cancelQueries({ queryKey: countBatchPrefix }),
    ])

    const applyDelta = (dir: number) => {
      queryClient.setQueryData<ShowSaveCount>(countQueryKey, (prev) =>
        prev
          ? {
              ...prev,
              save_count: Math.max(0, prev.save_count + dir),
              is_saved: dir > 0,
            }
          : prev
      )

      queryClient.setQueriesData<Record<string, SaveCountEntry>>(
        { queryKey: countBatchPrefix },
        (prev) => {
          const entry = prev?.[String(showId)]
          if (!prev || !entry) return prev
          return {
            ...prev,
            [String(showId)]: {
              save_count: Math.max(0, entry.save_count + dir),
              is_saved: dir > 0,
            },
          }
        }
      )
    }

    applyDelta(delta)

    try {
      if (isSaved) {
        await unsaveShow.mutateAsync(showId)
      } else {
        await saveShow.mutateAsync(showId)
      }
    } catch (error) {
      // Rollback on error
      applyDelta(-delta)
      throw error
    }
  }

  return {
    isLoading,
    toggle,
    error: saveShow.error || unsaveShow.error,
  }
}

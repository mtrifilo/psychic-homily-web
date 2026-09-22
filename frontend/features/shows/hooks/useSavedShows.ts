'use client'

import {
  type InfiniteData,
  useQuery,
  useInfiniteQuery,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys, createInvalidateQueries } from '@/lib/queryClient'
import { useAuthContext } from '@/lib/context/AuthContext'
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
  // The context viewer id verbatim. These keys scope a cache entry to a
  // viewer, so a caller that converts the id first writes a second entry for
  // the same reader and the prefix invalidations below stop matching it.
  userId?: string
  timeFilter?: 'upcoming' | 'past'
}

/**
 * A page of the viewer's saved shows.
 *
 * Holds THE WRITE-SIDE RULE itself rather than leaving it to callers (see
 * AuthStatus in lib/context/AuthContext): this key carries the viewer's
 * identity, and a request issued while the profile is in flight still sends
 * their cookie, so the response would be written under the viewer-less key and
 * outlive the session. A caller's own `enabled` cannot protect a key its
 * siblings and the optimistic mutations share.
 */
export const useSavedShows = (options: UseSavedShowsOptions = {}) => {
  const { limit = 50, offset = 0, enabled = true, userId, timeFilter } = options
  const { authStatus } = useAuthContext()

  const params = new URLSearchParams()
  params.set('limit', limit.toString())
  params.set('offset', offset.toString())
  if (timeFilter) params.set('time_filter', timeFilter)

  const endpoint = `${API_ENDPOINTS.SAVED_SHOWS.LIST}?${params.toString()}`

  return useQuery({
    queryKey: queryKeys.savedShows.list(userId, limit, offset, timeFilter),
    queryFn: async (): Promise<SavedShowsListResponse> => {
      return apiRequest<SavedShowsListResponse>(endpoint, {
        method: 'GET',
      })
    },
    enabled: enabled && authStatus !== 'pending' && !!userId,
    staleTime: 5 * 60 * 1000, // 5 minutes
  })
}

/**
 * How many saved shows a surface shows before sending the viewer to the full
 * list: Library's collapsed table, the signed-in home's module, and this
 * query's first page. One constant because those three are the SAME promise to
 * the viewer, and three literals cannot be kept equal by a comment.
 */
export const SAVED_SHOWS_COLLAPSED_COUNT = 4
const SAVED_SHOWS_NEXT_PAGE_SIZE = 100

/**
 * How many upcoming saves the signed-in home reads in one request.
 *
 * It is the API's maximum page size, not the four rows the module paints,
 * because the same response also supplies the ids the nearby list must leave
 * out. Reading only the displayed four would leave every later save eligible
 * to reappear in that list — and, since the saved list is ordered by date
 * across ALL cities, a viewer whose four soonest saves are elsewhere would
 * exclude nothing at all in the city they are looking at.
 *
 * The bound is therefore real but generous: a viewer holding more than this
 * many UPCOMING saves can still see one repeat.
 */
export const SAVED_SHOWS_HOME_READ_LIMIT = SAVED_SHOWS_NEXT_PAGE_SIZE

/**
 * Fetch a date-partitioned saved-show list incrementally. The first request is
 * the collapsed row count; expansion then uses the API's maximum page size so
 * large collections remain reachable without making the initial Library load
 * hydrate hundreds of hidden records.
 */
export const useInfiniteSavedShows = (
  timeFilter: 'upcoming' | 'past',
  userId: string | undefined,
  enabled: boolean = true
) =>
  useInfiniteQuery({
    queryKey: queryKeys.savedShows.infiniteList(userId, timeFilter),
    initialPageParam: { offset: 0, limit: SAVED_SHOWS_COLLAPSED_COUNT },
    queryFn: async ({ pageParam }): Promise<SavedShowsListResponse> => {
      const params = new URLSearchParams({
        limit: pageParam.limit.toString(),
        offset: pageParam.offset.toString(),
        time_filter: timeFilter,
      })

      return apiRequest<SavedShowsListResponse>(
        `${API_ENDPOINTS.SAVED_SHOWS.LIST}?${params.toString()}`,
        { method: 'GET' }
      )
    },
    getNextPageParam: (lastPage, pages) => {
      if (lastPage.shows.length === 0) return undefined
      const nextOffset = pages.reduce(
        (loaded, page) => loaded + page.shows.length,
        0
      )
      return nextOffset < lastPage.total
        ? { offset: nextOffset, limit: SAVED_SHOWS_NEXT_PAGE_SIZE }
        : undefined
    },
    enabled,
    staleTime: 5 * 60 * 1000,
  })

/**
 * Hook to fetch a single show's public save count (plus the caller's own
 * is_saved when authenticated). Uses optional auth, so it works logged-out.
 */
export const useShowSaveCount = (
  showId: number,
  isAuthenticated: boolean,
  enabled: boolean = true,
  userId?: string
) => {
  const { authStatus } = useAuthContext()
  return useQuery({
    queryKey: queryKeys.savedShows.count(
      showId,
      isAuthenticated,
      isAuthenticated ? userId : undefined
    ),
    queryFn: async (): Promise<ShowSaveCount> => {
      return apiRequest<ShowSaveCount>(API_ENDPOINTS.SAVE_COUNTS.SHOW(showId), {
        method: 'GET',
      })
    },
    // The count itself is public, so a settled-anonymous viewer still fetches;
    // this control paints what comes back. The pending term is the write-side
    // rule (see AuthStatus in lib/context/AuthContext).
    enabled:
      showId > 0 &&
      enabled &&
      authStatus !== 'pending' &&
      (!isAuthenticated || !!userId),
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
  isAuthenticated: boolean,
  userId?: string
) => {
  const { authStatus } = useAuthContext()
  return useQuery({
    queryKey: queryKeys.savedShows.countBatch(
      showIds,
      isAuthenticated,
      isAuthenticated ? userId : undefined
    ),
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
    // Same viewer-keyed hazard as the single-item read, on the highest-traffic
    // path: a list holds one of these for every row on screen.
    //
    // Disabling it cannot start a per-row request storm, by either of the two
    // routes a caller takes to `saveData`: `batchedSaveFor` maps an absent
    // result to 'pending' (ShowList, HomeShowList), and the chart call sites
    // pass a truthy zeroed fallback instead of undefined. `resolveBatchedSaveData`
    // reports `shouldSelfFetch: false` for both. A NEW caller that passes a bare
    // `data?.[id]` would break that, which is why the two safe shapes are named.
    enabled:
      showIds.length > 0 &&
      authStatus !== 'pending' &&
      (!isAuthenticated || !!userId),
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
    onSettled: () => {
      // Optional chart stats must reconcile even when the server committed but
      // the mutation response was lost. Never hold the core save pending on it.
      void invalidateQueries.personalCharts()
    },
  })
}

/**
 * Hook to unsave (remove) a show from user's list
 * Requires authentication
 */
interface UseUnsaveShowOptions {
  syncMode?: 'invalidate' | 'patch-infinite'
  userId?: string
}

export const useUnsaveShow = ({
  syncMode = 'invalidate',
  userId,
}: UseUnsaveShowOptions = {}) => {
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
    onSuccess: (_, showId) => {
      if (syncMode === 'invalidate') {
        // Re-sync the user's list and every cached save count from the server.
        invalidateQueries.savedShows()
        return
      }

      // Library infinite lists may contain many loaded pages. Patch the saved
      // row out locally instead of refetching every page in both date buckets.
      queryClient.setQueriesData<InfiniteData<SavedShowsListResponse>>(
        { queryKey: queryKeys.savedShows.infiniteListPrefix(userId) },
        data => {
          if (!data) return data
          let removed = false
          const pages = data.pages.map(page => {
            const shows = page.shows.filter(show => show.id !== showId)
            if (shows.length === page.shows.length) return page
            removed = true
            return { ...page, shows }
          })

          return removed
            ? {
                ...data,
                pages: pages.map(page => ({
                  ...page,
                  total: Math.max(0, page.total - 1),
                })),
              }
            : data
        }
      )

      // Other list shapes and save-count surfaces still need server truth,
      // but these narrow invalidations avoid reloading the infinite pages.
      void queryClient.invalidateQueries({
        queryKey: queryKeys.savedShows.listPrefix(userId),
      })
      void queryClient.invalidateQueries({
        queryKey: queryKeys.savedShows.countBatchPrefix(userId),
      })
      void queryClient.invalidateQueries({
        queryKey: ['savedShows', 'countBatch', false, null],
      })
      void queryClient.invalidateQueries({
        queryKey: queryKeys.savedShows.count(showId, true, userId),
      })
      void queryClient.invalidateQueries({
        queryKey: queryKeys.savedShows.count(showId, false),
      })
    },
    onSettled: () => {
      void invalidateQueries.personalCharts()
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
export const useSaveShowToggle = (
  showId: number,
  isSaved: boolean,
  userId?: string
) => {
  const queryClient = useQueryClient()
  const saveShow = useSaveShow()
  const unsaveShow = useUnsaveShow()

  const isLoading = saveShow.isPending || unsaveShow.isPending

  const toggle = async () => {
    // Toggling requires auth, so the authenticated variant of the key is the
    // only one that can be live for this user.
    const countQueryKey = queryKeys.savedShows.count(showId, true, userId)
    // Prefix filter: patches every cached batch, regardless of its show-id set
    // or auth flag, so a row's count moves the instant the heart is clicked.
    const countBatchPrefix = queryKeys.savedShows.countBatchPrefix(userId)
    const delta = isSaved ? -1 : 1

    // Cancel in-flight reads so stale responses don't overwrite the optimistic update
    await Promise.all([
      queryClient.cancelQueries({ queryKey: countQueryKey }),
      queryClient.cancelQueries({ queryKey: countBatchPrefix }),
    ])

    // Snapshot the exact prior values. Rollback restores them verbatim rather
    // than re-applying the inverse delta: `save_count` is clamped at 0, so
    // inverting -1 on an already-clamped 0 would resurrect a phantom +1.
    const previousCount = queryClient.getQueryData<ShowSaveCount>(countQueryKey)
    const previousBatches = queryClient.getQueriesData<
      Record<string, SaveCountEntry>
    >({ queryKey: countBatchPrefix })

    queryClient.setQueryData<ShowSaveCount>(countQueryKey, prev =>
      prev
        ? {
            ...prev,
            save_count: Math.max(0, prev.save_count + delta),
            is_saved: delta > 0,
          }
        : prev
    )

    queryClient.setQueriesData<Record<string, SaveCountEntry>>(
      { queryKey: countBatchPrefix },
      prev => {
        const entry = prev?.[String(showId)]
        if (!prev || !entry) return prev
        return {
          ...prev,
          [String(showId)]: {
            save_count: Math.max(0, entry.save_count + delta),
            is_saved: delta > 0,
          },
        }
      }
    )

    try {
      if (isSaved) {
        await unsaveShow.mutateAsync(showId)
      } else {
        await saveShow.mutateAsync(showId)
      }
    } catch (error) {
      // The single-show key holds only this show, so restoring it wholesale is
      // safe. A batch entry is SHARED by every show in the list, so restore
      // only this show's slot onto the CURRENT object — replacing the whole
      // snapshot would erase a sibling show's save that succeeded in the
      // meantime.
      queryClient.setQueryData(countQueryKey, previousCount)
      for (const [key, snapshot] of previousBatches) {
        queryClient.setQueryData<Record<string, SaveCountEntry>>(
          key,
          current => {
            const priorEntry = snapshot?.[String(showId)]
            if (!current || !priorEntry) return current
            return { ...current, [String(showId)]: priorEntry }
          }
        )
      }
      // The optimistic premise (`isSaved`) may itself have been stale — e.g. the
      // row was already unsaved from another tab. Restoring the snapshot only
      // undoes our guess, so re-sync from the server rather than trusting it.
      queryClient.invalidateQueries({ queryKey: queryKeys.savedShows.all })
      throw error
    }
  }

  return {
    isLoading,
    toggle,
    error: saveShow.error || unsaveShow.error,
  }
}

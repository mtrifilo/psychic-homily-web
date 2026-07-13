'use client'

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { releaseEndpoints, releaseQueryKeys } from '../api'
import type {
  BatchReleaseSaveCountsResponse,
  ReleaseSaveCount,
  ReleaseSaveCountEntry,
  ReleaseSaveResponse,
  SavedReleasesListResponse,
} from '../types'

export function useSavedReleases(
  limit: number,
  offset: number,
  userId: string | number | undefined
) {
  const params = new URLSearchParams({
    limit: String(limit),
    offset: String(offset),
  })
  return useQuery({
    queryKey: releaseQueryKeys.savedList(limit, offset, userId),
    queryFn: () =>
      apiRequest<SavedReleasesListResponse>(
        `${releaseEndpoints.SAVED_LIST}?${params.toString()}`,
        { method: 'GET' }
      ),
    enabled: userId !== undefined,
  })
}

export function useReleaseSaveCount(
  releaseId: number,
  isAuthenticated: boolean,
  enabled = true,
  userId?: string | number
) {
  return useQuery({
    queryKey: releaseQueryKeys.saveCount(releaseId, isAuthenticated, userId),
    queryFn: () =>
      apiRequest<ReleaseSaveCount>(releaseEndpoints.SAVE_COUNT(releaseId), {
        method: 'GET',
      }),
    enabled: releaseId > 0 && enabled,
    staleTime: 2 * 60 * 1000,
  })
}

export function useReleaseSaveCountBatch(
  releaseIds: number[],
  isAuthenticated: boolean,
  userId?: string | number
) {
  return useQuery({
    queryKey: releaseQueryKeys.saveCountBatch(
      releaseIds,
      isAuthenticated,
      userId
    ),
    queryFn: async (): Promise<Record<string, ReleaseSaveCountEntry>> => {
      const response = await apiRequest<BatchReleaseSaveCountsResponse>(
        releaseEndpoints.SAVE_COUNTS_BATCH,
        {
          method: 'POST',
          body: JSON.stringify({ release_ids: releaseIds }),
        }
      )
      return response.saves
    },
    enabled: releaseIds.length > 0,
    staleTime: 2 * 60 * 1000,
  })
}

export function useReleaseSaveToggle(
  releaseId: number,
  isSaved: boolean,
  userId?: string | number
) {
  const queryClient = useQueryClient()
  const save = useMutation({
    mutationFn: () =>
      apiRequest<ReleaseSaveResponse>(releaseEndpoints.SAVE(releaseId), {
        method: 'POST',
      }),
  })
  const unsave = useMutation({
    mutationFn: () =>
      apiRequest<ReleaseSaveResponse>(releaseEndpoints.UNSAVE(releaseId), {
        method: 'DELETE',
      }),
  })

  const toggle = async () => {
    const singleKey = releaseQueryKeys.saveCount(releaseId, true, userId)
    const batchPrefix = releaseQueryKeys.saveCountBatchPrefix(userId)
    const delta = isSaved ? -1 : 1

    await Promise.all([
      queryClient.cancelQueries({ queryKey: singleKey }),
      queryClient.cancelQueries({ queryKey: batchPrefix }),
    ])
    const previousSingle = queryClient.getQueryData<ReleaseSaveCount>(singleKey)
    const previousBatches = queryClient.getQueriesData<
      Record<string, ReleaseSaveCountEntry>
    >({ queryKey: batchPrefix })

    queryClient.setQueryData<ReleaseSaveCount>(singleKey, previous =>
      previous
        ? {
            ...previous,
            save_count: Math.max(0, previous.save_count + delta),
            is_saved: delta > 0,
          }
        : previous
    )
    queryClient.setQueriesData<Record<string, ReleaseSaveCountEntry>>(
      { queryKey: batchPrefix },
      previous => {
        const entry = previous?.[String(releaseId)]
        if (!previous || !entry) return previous
        return {
          ...previous,
          [String(releaseId)]: {
            save_count: Math.max(0, entry.save_count + delta),
            is_saved: delta > 0,
          },
        }
      }
    )

    try {
      if (isSaved) await unsave.mutateAsync()
      else await save.mutateAsync()
    } catch (error) {
      queryClient.setQueryData(singleKey, previousSingle)
      for (const [key, snapshot] of previousBatches) {
        queryClient.setQueryData<Record<string, ReleaseSaveCountEntry>>(
          key,
          current => {
            const priorEntry = snapshot?.[String(releaseId)]
            if (!current || !priorEntry) return current
            return { ...current, [String(releaseId)]: priorEntry }
          }
        )
      }
      throw error
    } finally {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['releases', 'saved'] }),
        queryClient.invalidateQueries({ queryKey: singleKey }),
        queryClient.invalidateQueries({ queryKey: batchPrefix }),
      ])
    }
  }

  return {
    toggle,
    isLoading: save.isPending || unsave.isPending,
    error: save.error ?? unsave.error,
  }
}

'use client'

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys, createInvalidateQueries } from '@/lib/queryClient'
// Note: useAttendance uses ATTENDANCE endpoints from lib/api (not show-specific)
import { useAuthContext } from '@/lib/context/AuthContext'
import type {
  AttendanceCounts,
  BatchAttendanceResponse,
  MyShowsResponse,
} from '../types'

/**
 * Hook to fetch attendance counts + user status for a single show.
 * Uses optional auth — if authenticated, includes user's status.
 */
export const useShowAttendance = (showId: number) => {
  return useQuery({
    queryKey: queryKeys.attendance.show(showId),
    queryFn: async (): Promise<AttendanceCounts> => {
      return apiRequest<AttendanceCounts>(
        API_ENDPOINTS.ATTENDANCE.SHOW(showId),
        { method: 'GET' }
      )
    },
    enabled: showId > 0,
    staleTime: 2 * 60 * 1000, // 2 minutes
  })
}

/**
 * Hook to fetch batch attendance data for multiple shows.
 * Uses POST to send the list of show IDs.
 * Returns a map of show_id (string) -> AttendanceCounts.
 */
export const useBatchAttendance = (showIds: number[]) => {
  return useQuery({
    queryKey: queryKeys.attendance.batch(showIds),
    queryFn: async (): Promise<Record<string, AttendanceCounts>> => {
      const response = await apiRequest<BatchAttendanceResponse>(
        API_ENDPOINTS.ATTENDANCE.BATCH,
        {
          method: 'POST',
          body: JSON.stringify({ show_ids: showIds }),
        }
      )
      return response.attendance
    },
    enabled: showIds.length > 0,
    staleTime: 2 * 60 * 1000,
  })
}

/**
 * Hook to set attendance status (going or interested) for a show.
 * Includes optimistic updates for snappy UX.
 */
export const useSetAttendance = () => {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async ({
      showId,
      status,
    }: {
      showId: number
      status: 'going' | 'interested'
    }): Promise<{ success: boolean; message: string }> => {
      return apiRequest(API_ENDPOINTS.ATTENDANCE.SHOW(showId), {
        method: 'POST',
        body: JSON.stringify({ status }),
      })
    },
    onMutate: async ({ showId, status }) => {
      // Cancel outgoing queries for this show's attendance
      await queryClient.cancelQueries({
        queryKey: queryKeys.attendance.show(showId),
      })

      // Snapshot previous value
      const previousData = queryClient.getQueryData<AttendanceCounts>(
        queryKeys.attendance.show(showId)
      )

      // Optimistically update the single show attendance cache
      if (previousData) {
        const oldStatus = previousData.user_status
        const updated = { ...previousData }

        // Decrement old status count
        if (oldStatus === 'going') updated.going_count = Math.max(0, updated.going_count - 1)
        if (oldStatus === 'interested') updated.interested_count = Math.max(0, updated.interested_count - 1)

        // Increment new status count
        if (status === 'going') updated.going_count += 1
        if (status === 'interested') updated.interested_count += 1

        updated.user_status = status

        queryClient.setQueryData(queryKeys.attendance.show(showId), updated)
      }

      return { previousData }
    },
    onError: (_err, { showId }, context) => {
      // Rollback on error
      if (context?.previousData) {
        queryClient.setQueryData(
          queryKeys.attendance.show(showId),
          context.previousData
        )
      }
    },
    onSettled: (_data, _error, { showId }) => {
      // Refetch to ensure consistency
      queryClient.invalidateQueries({
        queryKey: queryKeys.attendance.show(showId),
      })
      // Invalidate batch and my-shows queries
      invalidateQueries.attendance()
    },
  })
}

/**
 * Hook to remove attendance for a show.
 * Includes optimistic updates for snappy UX.
 */
export const useRemoveAttendance = () => {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async (
      showId: number
    ): Promise<{ success: boolean; message: string }> => {
      return apiRequest(API_ENDPOINTS.ATTENDANCE.SHOW(showId), {
        method: 'DELETE',
      })
    },
    onMutate: async (showId) => {
      await queryClient.cancelQueries({
        queryKey: queryKeys.attendance.show(showId),
      })

      const previousData = queryClient.getQueryData<AttendanceCounts>(
        queryKeys.attendance.show(showId)
      )

      if (previousData) {
        const updated = { ...previousData }
        const oldStatus = previousData.user_status

        if (oldStatus === 'going') updated.going_count = Math.max(0, updated.going_count - 1)
        if (oldStatus === 'interested') updated.interested_count = Math.max(0, updated.interested_count - 1)

        updated.user_status = ''

        queryClient.setQueryData(queryKeys.attendance.show(showId), updated)
      }

      return { previousData }
    },
    onError: (_err, showId, context) => {
      if (context?.previousData) {
        queryClient.setQueryData(
          queryKeys.attendance.show(showId),
          context.previousData
        )
      }
    },
    onSettled: (_data, _error, showId) => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.attendance.show(showId),
      })
      invalidateQueries.attendance()
    },
  })
}

interface UseMyShowsOptions {
  status?: string
  limit?: number
  offset?: number
}

/**
 * Hook to fetch the authenticated user's attending shows.
 * Only enabled when user is authenticated.
 */
export const useMyShows = (options: UseMyShowsOptions = {}) => {
  const { isAuthenticated } = useAuthContext()
  const { status = 'all', limit = 20, offset = 0 } = options

  const params = new URLSearchParams()
  if (status && status !== 'all') params.set('status', status)
  params.set('limit', limit.toString())
  params.set('offset', offset.toString())

  const endpoint = `${API_ENDPOINTS.ATTENDANCE.MY_SHOWS}?${params.toString()}`

  return useQuery({
    queryKey: queryKeys.attendance.myShows({ status, limit, offset }),
    queryFn: async (): Promise<MyShowsResponse> => {
      return apiRequest<MyShowsResponse>(endpoint, { method: 'GET' })
    },
    enabled: isAuthenticated,
    staleTime: 2 * 60 * 1000,
  })
}

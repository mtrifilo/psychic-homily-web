'use client'

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../api'
import { createInvalidateQueries } from '../queryClient'
import type {
  ShowResponse,
  PendingShowsResponse,
  RejectedShowsResponse,
  ApproveShowRequest,
  RejectShowRequest,
} from '../types/show'

/**
 * Query key factory for admin queries
 */
export const adminQueryKeys = {
  pendingShows: (limit: number, offset: number) =>
    ['admin', 'shows', 'pending', { limit, offset }] as const,
  rejectedShows: (limit: number, offset: number, search?: string) =>
    ['admin', 'shows', 'rejected', { limit, offset, search }] as const,
}

/**
 * Hook for fetching pending shows (admin only)
 */
export function usePendingShows(options?: { limit?: number; offset?: number }) {
  const limit = options?.limit ?? 50
  const offset = options?.offset ?? 0

  return useQuery({
    queryKey: adminQueryKeys.pendingShows(limit, offset),
    queryFn: async () => {
      const params = new URLSearchParams({
        limit: limit.toString(),
        offset: offset.toString(),
      })
      return apiRequest<PendingShowsResponse>(
        `${API_ENDPOINTS.ADMIN.SHOWS.PENDING}?${params}`
      )
    },
    staleTime: 30 * 1000, // 30 seconds - shorter for admin data
  })
}

/**
 * Hook for fetching rejected shows (admin only)
 */
export function useRejectedShows(options?: {
  limit?: number
  offset?: number
  search?: string
}) {
  const limit = options?.limit ?? 50
  const offset = options?.offset ?? 0
  const search = options?.search

  return useQuery({
    queryKey: adminQueryKeys.rejectedShows(limit, offset, search),
    queryFn: async () => {
      const params = new URLSearchParams({
        limit: limit.toString(),
        offset: offset.toString(),
      })
      if (search) {
        params.set('search', search)
      }
      return apiRequest<RejectedShowsResponse>(
        `${API_ENDPOINTS.ADMIN.SHOWS.REJECTED}?${params}`
      )
    },
    staleTime: 30 * 1000, // 30 seconds - shorter for admin data
  })
}

/**
 * Hook for approving a pending show (admin only)
 */
export function useApproveShow() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async ({
      showId,
      verifyVenues,
    }: {
      showId: number
      verifyVenues: boolean
    }) => {
      const body: ApproveShowRequest = { verify_venues: verifyVenues }
      return apiRequest<ShowResponse>(
        API_ENDPOINTS.ADMIN.SHOWS.APPROVE(showId),
        {
          method: 'POST',
          body: JSON.stringify(body),
        }
      )
    },
    onSuccess: () => {
      // Invalidate pending shows list
      queryClient.invalidateQueries({ queryKey: ['admin', 'shows', 'pending'] })
      // Invalidate public shows list since a show was approved
      invalidateQueries.shows()
      // Invalidate admin stats (dashboard counts)
      queryClient.invalidateQueries({ queryKey: ['admin', 'stats'] })
    },
  })
}

/**
 * Hook for rejecting a pending show (admin only)
 */
export function useRejectShow() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async ({
      showId,
      reason,
    }: {
      showId: number
      reason: string
    }) => {
      const body: RejectShowRequest = { reason }
      return apiRequest<ShowResponse>(
        API_ENDPOINTS.ADMIN.SHOWS.REJECT(showId),
        {
          method: 'POST',
          body: JSON.stringify(body),
        }
      )
    },
    onSuccess: () => {
      // Invalidate pending shows list
      queryClient.invalidateQueries({ queryKey: ['admin', 'shows', 'pending'] })
      // Invalidate admin stats (dashboard counts)
      queryClient.invalidateQueries({ queryKey: ['admin', 'stats'] })
    },
  })
}

/**
 * Hook for setting a show's sold out status (admin or submitter)
 */
export function useSetShowSoldOut() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async ({
      showId,
      value,
    }: {
      showId: number
      value: boolean
    }) => {
      return apiRequest<ShowResponse>(
        API_ENDPOINTS.SHOWS.SET_SOLD_OUT(showId),
        {
          method: 'POST',
          body: JSON.stringify({ value }),
        }
      )
    },
    onSuccess: () => {
      // Invalidate shows list since display may change
      invalidateQueries.shows()
    },
  })
}

/**
 * Hook for setting a show's cancelled status (admin or submitter)
 */
export function useSetShowCancelled() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async ({
      showId,
      value,
    }: {
      showId: number
      value: boolean
    }) => {
      return apiRequest<ShowResponse>(
        API_ENDPOINTS.SHOWS.SET_CANCELLED(showId),
        {
          method: 'POST',
          body: JSON.stringify({ value }),
        }
      )
    },
    onSuccess: () => {
      // Invalidate shows list since display may change
      invalidateQueries.shows()
    },
  })
}

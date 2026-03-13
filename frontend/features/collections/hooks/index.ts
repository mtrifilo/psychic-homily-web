'use client'

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
import type { Collection, CollectionDetail, CollectionStats } from '../types'

export function useCollections(options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: queryKeys.collections.all,
    queryFn: async () => {
      return apiRequest<{ collections: Collection[]; total: number }>(
        API_ENDPOINTS.COLLECTIONS.LIST
      )
    },
    enabled: options?.enabled ?? true,
  })
}

export function useMyCollections(options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: queryKeys.collections.my,
    queryFn: async () => {
      return apiRequest<{ collections: Collection[]; total: number }>(
        API_ENDPOINTS.COLLECTIONS.MY
      )
    },
    enabled: options?.enabled ?? true,
  })
}

export function useCollection(slug: string, options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: queryKeys.collections.detail(slug),
    queryFn: async () => {
      return apiRequest<CollectionDetail>(
        API_ENDPOINTS.COLLECTIONS.DETAIL(slug)
      )
    },
    enabled: (options?.enabled ?? true) && slug.length > 0,
  })
}

export function useCollectionStats(slug: string, options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: queryKeys.collections.stats(slug),
    queryFn: async () => {
      return apiRequest<CollectionStats>(
        API_ENDPOINTS.COLLECTIONS.STATS(slug)
      )
    },
    enabled: (options?.enabled ?? true) && slug.length > 0,
  })
}

export function useSetFeatured() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({ slug, featured }: { slug: string; featured: boolean }) => {
      return apiRequest<void>(API_ENDPOINTS.COLLECTIONS.FEATURE(slug), {
        method: 'PUT',
        body: JSON.stringify({ featured }),
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.collections.all })
    },
  })
}

export function useDeleteCollection() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({ slug }: { slug: string }) => {
      return apiRequest<void>(API_ENDPOINTS.COLLECTIONS.DETAIL(slug), {
        method: 'DELETE',
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.collections.all })
      queryClient.invalidateQueries({ queryKey: queryKeys.collections.my })
    },
  })
}

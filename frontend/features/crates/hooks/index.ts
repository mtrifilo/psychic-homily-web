'use client'

/**
 * Crate Hooks
 *
 * TanStack Query hooks for crate CRUD, items, and subscriptions.
 */

import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
import type { Crate, CrateDetail, CrateStats } from '../types'

// ──────────────────────────────────────────────
// Queries
// ──────────────────────────────────────────────

/** Fetch public crates list */
export function useCrates() {
  return useQuery({
    queryKey: queryKeys.crates.all,
    queryFn: () =>
      apiRequest<{ crates: Crate[]; total: number }>(
        API_ENDPOINTS.CRATES.LIST
      ),
    staleTime: 5 * 60 * 1000,
    placeholderData: keepPreviousData,
  })
}

/** Fetch a single crate by slug (includes items) */
export function useCrate(slug: string, options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: queryKeys.crates.detail(slug),
    queryFn: () =>
      apiRequest<CrateDetail>(API_ENDPOINTS.CRATES.DETAIL(slug)),
    enabled: (options?.enabled ?? true) && slug.length > 0,
    staleTime: 5 * 60 * 1000,
  })
}

/** Fetch crate stats */
export function useCrateStats(slug: string, options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: queryKeys.crates.stats(slug),
    queryFn: () =>
      apiRequest<CrateStats>(API_ENDPOINTS.CRATES.STATS(slug)),
    enabled: (options?.enabled ?? true) && slug.length > 0,
  })
}

/** Fetch the authenticated user's own crates */
export function useMyCrates() {
  return useQuery({
    queryKey: queryKeys.crates.my,
    queryFn: () =>
      apiRequest<{ crates: Crate[]; total: number }>(
        API_ENDPOINTS.CRATES.MY
      ),
    staleTime: 5 * 60 * 1000,
  })
}

// ──────────────────────────────────────────────
// Mutations
// ──────────────────────────────────────────────

/** Toggle featured status on a crate (admin) */
export function useSetFeatured() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ slug, featured }: { slug: string; featured: boolean }) =>
      apiRequest<void>(API_ENDPOINTS.CRATES.FEATURE(slug), {
        method: 'PUT',
        body: JSON.stringify({ featured }),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.crates.all })
    },
  })
}

/** Create a new crate */
export function useCreateCrate() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: {
      title: string
      description?: string
      is_public: boolean
      collaborative: boolean
    }) =>
      apiRequest<CrateDetail>(API_ENDPOINTS.CRATES.LIST, {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.crates.all })
      queryClient.invalidateQueries({ queryKey: queryKeys.crates.my })
    },
  })
}

/** Update an existing crate */
export function useUpdateCrate() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({
      slug,
      ...data
    }: {
      slug: string
      title?: string
      description?: string
      is_public?: boolean
      collaborative?: boolean
    }) =>
      apiRequest<CrateDetail>(API_ENDPOINTS.CRATES.DETAIL(slug), {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.crates.all })
      queryClient.invalidateQueries({
        queryKey: queryKeys.crates.detail(variables.slug),
      })
      queryClient.invalidateQueries({ queryKey: queryKeys.crates.my })
    },
  })
}

/** Delete a crate */
export function useDeleteCrate() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ slug }: { slug: string }) =>
      apiRequest<void>(API_ENDPOINTS.CRATES.DETAIL(slug), {
        method: 'DELETE',
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.crates.all })
      queryClient.invalidateQueries({ queryKey: queryKeys.crates.my })
    },
  })
}

/** Add an item to a crate */
export function useAddCrateItem() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({
      slug,
      entityType,
      entityId,
      notes,
    }: {
      slug: string
      entityType: string
      entityId: number
      notes?: string
    }) =>
      apiRequest<void>(API_ENDPOINTS.CRATES.ITEMS(slug), {
        method: 'POST',
        body: JSON.stringify({
          entity_type: entityType,
          entity_id: entityId,
          notes,
        }),
      }),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.crates.detail(variables.slug),
      })
    },
  })
}

/** Remove an item from a crate */
export function useRemoveCrateItem() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ slug, itemId }: { slug: string; itemId: number }) =>
      apiRequest<void>(API_ENDPOINTS.CRATES.ITEM(slug, itemId), {
        method: 'DELETE',
      }),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.crates.detail(variables.slug),
      })
    },
  })
}

/** Subscribe to a crate */
export function useSubscribeCrate() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ slug }: { slug: string }) =>
      apiRequest<void>(API_ENDPOINTS.CRATES.SUBSCRIBE(slug), {
        method: 'POST',
      }),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.crates.detail(variables.slug),
      })
    },
  })
}

/** Unsubscribe from a crate */
export function useUnsubscribeCrate() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ slug }: { slug: string }) =>
      apiRequest<void>(API_ENDPOINTS.CRATES.SUBSCRIBE(slug), {
        method: 'DELETE',
      }),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.crates.detail(variables.slug),
      })
    },
  })
}

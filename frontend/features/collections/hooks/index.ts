'use client'

/**
 * Collection Hooks
 *
 * TanStack Query hooks for collection CRUD, items, and subscriptions.
 */

import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
import type {
  Collection,
  CollectionDetail,
  CollectionDisplayMode,
  CollectionStats,
} from '../types'

// ──────────────────────────────────────────────
// Queries
// ──────────────────────────────────────────────

/** Filter params for the public collections list */
export interface CollectionListParams {
  search?: string
  featured?: boolean
  entityType?: string
}

/** Fetch public collections list with optional filters */
export function useCollections(params?: CollectionListParams) {
  return useQuery({
    queryKey: queryKeys.collections.list(params ? { ...params } : undefined),
    queryFn: () => {
      const searchParams = new URLSearchParams()
      if (params?.search) searchParams.set('search', params.search)
      if (params?.featured) searchParams.set('featured', '1')
      if (params?.entityType) searchParams.set('entity_type', params.entityType)

      const qs = searchParams.toString()
      const url = qs
        ? `${API_ENDPOINTS.COLLECTIONS.LIST}?${qs}`
        : API_ENDPOINTS.COLLECTIONS.LIST

      return apiRequest<{ collections: Collection[]; total: number }>(url).then(
        (data) => ({
          collections: data.collections ?? [],
          total: data.total,
        })
      )
    },
    staleTime: 5 * 60 * 1000,
    placeholderData: keepPreviousData,
  })
}

/** Fetch a single collection by slug (includes items) */
export function useCollection(slug: string, options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: queryKeys.collections.detail(slug),
    queryFn: () =>
      apiRequest<CollectionDetail>(API_ENDPOINTS.COLLECTIONS.DETAIL(slug)),
    enabled: (options?.enabled ?? true) && slug.length > 0,
    staleTime: 5 * 60 * 1000,
  })
}

/** Fetch collection stats */
export function useCollectionStats(slug: string, options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: queryKeys.collections.stats(slug),
    queryFn: () =>
      apiRequest<CollectionStats>(API_ENDPOINTS.COLLECTIONS.STATS(slug)),
    enabled: (options?.enabled ?? true) && slug.length > 0,
  })
}

/** Fetch the authenticated user's own collections */
export function useMyCollections() {
  return useQuery({
    queryKey: queryKeys.collections.my,
    queryFn: () =>
      apiRequest<{ collections: Collection[]; total: number }>(
        API_ENDPOINTS.COLLECTIONS.MY
      ).then((data) => ({
        collections: data.collections ?? [],
        total: data.total,
      })),
    staleTime: 5 * 60 * 1000,
  })
}

// ──────────────────────────────────────────────
// Mutations
// ──────────────────────────────────────────────

/** Toggle featured status on a collection (admin) */
export function useSetFeatured() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ slug, featured }: { slug: string; featured: boolean }) =>
      apiRequest<void>(API_ENDPOINTS.COLLECTIONS.FEATURE(slug), {
        method: 'PUT',
        body: JSON.stringify({ featured }),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.collections.all })
    },
  })
}

/** Create a new collection */
export function useCreateCollection() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: {
      title: string
      description?: string
      is_public: boolean
      collaborative: boolean
      display_mode?: CollectionDisplayMode
    }) =>
      apiRequest<CollectionDetail>(API_ENDPOINTS.COLLECTIONS.LIST, {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.collections.all })
      queryClient.invalidateQueries({ queryKey: queryKeys.collections.my })
    },
  })
}

/** Update an existing collection */
export function useUpdateCollection() {
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
      display_mode?: CollectionDisplayMode
    }) =>
      apiRequest<CollectionDetail>(API_ENDPOINTS.COLLECTIONS.DETAIL(slug), {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.collections.all })
      queryClient.invalidateQueries({
        queryKey: queryKeys.collections.detail(variables.slug),
      })
      queryClient.invalidateQueries({ queryKey: queryKeys.collections.my })
    },
  })
}

/** Delete a collection */
export function useDeleteCollection() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ slug }: { slug: string }) =>
      apiRequest<void>(API_ENDPOINTS.COLLECTIONS.DETAIL(slug), {
        method: 'DELETE',
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.collections.all })
      queryClient.invalidateQueries({ queryKey: queryKeys.collections.my })
    },
  })
}

/** Add an item to a collection */
export function useAddCollectionItem() {
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
      apiRequest<void>(API_ENDPOINTS.COLLECTIONS.ITEMS(slug), {
        method: 'POST',
        body: JSON.stringify({
          entity_type: entityType,
          entity_id: entityId,
          notes,
        }),
      }),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.collections.detail(variables.slug),
      })
    },
  })
}

/** Remove an item from a collection */
export function useRemoveCollectionItem() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ slug, itemId }: { slug: string; itemId: number }) =>
      apiRequest<void>(API_ENDPOINTS.COLLECTIONS.ITEM(slug, itemId), {
        method: 'DELETE',
      }),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.collections.detail(variables.slug),
      })
    },
  })
}

/** Reorder items in a collection */
export function useReorderCollectionItems() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({
      slug,
      items,
    }: {
      slug: string
      items: { item_id: number; position: number }[]
    }) =>
      apiRequest<void>(API_ENDPOINTS.COLLECTIONS.REORDER(slug), {
        method: 'PUT',
        body: JSON.stringify({ items }),
      }),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.collections.detail(variables.slug),
      })
    },
  })
}

/** Update an item's notes in a collection */
export function useUpdateCollectionItem() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({
      slug,
      itemId,
      notes,
    }: {
      slug: string
      itemId: number
      notes: string | null
    }) =>
      apiRequest<void>(API_ENDPOINTS.COLLECTIONS.UPDATE_ITEM(slug, itemId), {
        method: 'PATCH',
        body: JSON.stringify({ notes }),
      }),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.collections.detail(variables.slug),
      })
    },
  })
}

/** Subscribe to a collection */
export function useSubscribeCollection() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ slug }: { slug: string }) =>
      apiRequest<void>(API_ENDPOINTS.COLLECTIONS.SUBSCRIBE(slug), {
        method: 'POST',
      }),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.collections.detail(variables.slug),
      })
    },
  })
}

/** Unsubscribe from a collection */
export function useUnsubscribeCollection() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ slug }: { slug: string }) =>
      apiRequest<void>(API_ENDPOINTS.COLLECTIONS.SUBSCRIBE(slug), {
        method: 'DELETE',
      }),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.collections.detail(variables.slug),
      })
    },
  })
}

/** Fetch public collections containing a specific entity */
export function useEntityCollections(
  entityType: string,
  entityId: number,
  options?: { enabled?: boolean }
) {
  return useQuery({
    queryKey: queryKeys.collections.entity(entityType, entityId),
    queryFn: () =>
      apiRequest<{ collections: Collection[]; }>(
        API_ENDPOINTS.COLLECTIONS.ENTITY(entityType, entityId)
      ).then((data) => ({
        collections: data.collections ?? [],
      })),
    enabled: (options?.enabled ?? true) && entityId > 0,
    staleTime: 5 * 60 * 1000,
  })
}

/** Fetch a user's public collections (for profile pages) */
export function useUserPublicCollections(
  username: string,
  options?: { enabled?: boolean }
) {
  return useQuery({
    queryKey: queryKeys.collections.userPublic(username),
    queryFn: () =>
      apiRequest<{ collections: Collection[]; total: number }>(
        API_ENDPOINTS.COLLECTIONS.USER_PUBLIC(username)
      ).then((data) => ({
        collections: data.collections ?? [],
        total: data.total,
      })),
    enabled: (options?.enabled ?? true) && username.length > 0,
    staleTime: 5 * 60 * 1000,
  })
}

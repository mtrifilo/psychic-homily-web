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
  AddCollectionTagResponse,
  Collection,
  CollectionDetail,
  CollectionDisplayMode,
  CollectionGraphResponse,
  CollectionStats,
} from '../types'

// ──────────────────────────────────────────────
// Queries
// ──────────────────────────────────────────────

/** Sort values understood by the public collections list endpoint. PSY-352. */
export type CollectionListSort = 'popular'

/** Filter params for the public collections list */
export interface CollectionListParams {
  search?: string
  featured?: boolean
  entityType?: string
  /**
   * PSY-352: when set to "popular", the server orders results by
   * HN gravity (likes / age^1.8). Omit for default (recently-updated).
   */
  sort?: CollectionListSort
  /**
   * PSY-354: filter to collections tagged with this slug. Single-tag for
   * the MVP — multi-tag intersection isn't supported yet on the server.
   */
  tag?: string
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
      if (params?.sort) searchParams.set('sort', params.sort)
      if (params?.tag) searchParams.set('tag', params.tag)

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

/**
 * Fetch the artist-relationship subgraph for a collection's artist items.
 * PSY-366. Empty `types` (or omitted) returns all allowed edge types
 * (shared_bills, shared_label, member_of, side_project, similar,
 * radio_cooccurrence). Returns a 200 with empty nodes/links if the
 * collection has no artist items.
 */
export function useCollectionGraph(options: {
  slug: string
  types?: string[]
  enabled?: boolean
}) {
  const { slug, types, enabled = true } = options
  const params = new URLSearchParams()
  if (types && types.length > 0) {
    params.set('types', types.join(','))
  }
  const qs = params.toString()
  const endpoint = qs
    ? `${API_ENDPOINTS.COLLECTIONS.GRAPH(slug)}?${qs}`
    : API_ENDPOINTS.COLLECTIONS.GRAPH(slug)

  return useQuery({
    queryKey: queryKeys.collections.graph(slug, types),
    queryFn: () => apiRequest<CollectionGraphResponse>(endpoint),
    enabled: enabled && slug.length > 0,
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

/**
 * PSY-359: list of the authenticated user's own collection IDs that already
 * contain the given entity. Backs the pre-check state on the multi-select
 * Add-to-Collection popover so it can render the right boxes ticked in a
 * single round-trip (no N+1 contains-check fan-out across cards).
 *
 * Returns a `Set<number>` (constructed once per response) so callers can
 * check membership in O(1) without re-allocating per render. `enabled`
 * defaults to `true`; pass `false` to defer the request until the popover
 * opens — saves a fetch on every entity page render.
 */
export function useUserCollectionsContaining(
  entityType: string,
  entityId: number,
  options?: { enabled?: boolean }
) {
  return useQuery({
    queryKey: queryKeys.collections.containing(entityType, entityId),
    queryFn: async () => {
      const url = `${API_ENDPOINTS.COLLECTIONS.CONTAINS}?entity_type=${encodeURIComponent(entityType)}&entity_id=${entityId}`
      const data = await apiRequest<{ collection_ids: number[] }>(url)
      return new Set<number>(data.collection_ids ?? [])
    },
    enabled: (options?.enabled ?? true) && entityId > 0,
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
      /**
       * Cover image URL. PSY-371. Send `null` (or empty string) to clear an
       * existing cover; send a URL string to set one. Backend already
       * accepts the field on PUT; the UI was the only missing piece.
       */
      cover_image_url?: string | null
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

/**
 * Clone (fork) an existing collection. PSY-351.
 *
 * The new collection is owned by the authenticated caller, copies the
 * source's items + notes + positions, and carries `forked_from_collection_id`
 * back to the source so the detail page can render inline attribution.
 */
export function useCloneCollection() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ slug }: { slug: string }) =>
      apiRequest<CollectionDetail>(API_ENDPOINTS.COLLECTIONS.CLONE(slug), {
        method: 'POST',
      }),
    onSuccess: (newCollection, variables) => {
      // The original collection's `forks_count` just incremented; the
      // user's collection list gained an entry.
      queryClient.invalidateQueries({ queryKey: queryKeys.collections.all })
      queryClient.invalidateQueries({ queryKey: queryKeys.collections.my })
      // Source detail (if cached) needs the bumped forks_count.
      queryClient.invalidateQueries({
        queryKey: queryKeys.collections.detail(variables.slug),
      })
      // Pre-warm the new collection in cache so navigation is instant.
      queryClient.setQueryData(
        queryKeys.collections.detail(newCollection.slug),
        newCollection
      )
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
      // PSY-359: pre-check answer for this entity is now stale — the popover
      // should show the new collection as already-containing on next open.
      queryClient.invalidateQueries({
        queryKey: queryKeys.collections.containing(
          variables.entityType,
          variables.entityId
        ),
      })
      // Public "appears in" backlinks on the entity page also need to refresh
      // (if the collection is public).
      queryClient.invalidateQueries({
        queryKey: queryKeys.collections.entity(
          variables.entityType,
          variables.entityId
        ),
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

// ──────────────────────────────────────────────
// Likes (PSY-352)
// ──────────────────────────────────────────────

/**
 * Server-shaped response for POST/DELETE on the /like endpoint. PSY-352.
 * Mirrors backend `contracts.CollectionLikeResponse` exactly.
 */
export interface CollectionLikeResponse {
  like_count: number
  user_likes_this: boolean
}

/**
 * Toggle helper used by the like + unlike mutations to optimistically patch
 * the cached detail entry without a full refetch. Keeps the heart icon in
 * sync the moment the user clicks. The server response confirms the count
 * on success; on error TanStack rolls back via onError.
 *
 * Detail cache: `queryKeys.collections.detail(slug)` holds a `CollectionDetail`.
 * We update its `like_count` and `user_likes_this` in place.
 */
function patchDetailLikeCache(
  queryClient: ReturnType<typeof useQueryClient>,
  slug: string,
  liking: boolean
): CollectionDetail | undefined {
  const key = queryKeys.collections.detail(slug)
  const previous = queryClient.getQueryData<CollectionDetail>(key)
  if (!previous) return previous

  // Don't double-count: if the optimistic state already matches the action,
  // leave the cache alone. The server will return the authoritative count.
  if (previous.user_likes_this === liking) return previous

  const next: CollectionDetail = {
    ...previous,
    user_likes_this: liking,
    like_count: Math.max(0, previous.like_count + (liking ? 1 : -1)),
  }
  queryClient.setQueryData(key, next)
  return previous
}

/**
 * Like a collection. Optimistically updates the detail cache (heart fills
 * + count increments) and rolls back on error. Server response replaces the
 * optimistic state on success.
 */
export function useLikeCollection() {
  const queryClient = useQueryClient()
  return useMutation<
    CollectionLikeResponse,
    Error,
    { slug: string },
    { previousDetail: CollectionDetail | undefined }
  >({
    mutationFn: ({ slug }) =>
      apiRequest<CollectionLikeResponse>(API_ENDPOINTS.COLLECTIONS.LIKE(slug), {
        method: 'POST',
      }),
    onMutate: async ({ slug }) => {
      await queryClient.cancelQueries({
        queryKey: queryKeys.collections.detail(slug),
      })
      const previousDetail = patchDetailLikeCache(queryClient, slug, true)
      return { previousDetail }
    },
    onError: (_err, variables, context) => {
      // Rollback to the pre-mutation snapshot.
      if (context?.previousDetail) {
        queryClient.setQueryData(
          queryKeys.collections.detail(variables.slug),
          context.previousDetail
        )
      }
    },
    onSuccess: (data, variables) => {
      // Server is the source of truth for the count — replace the optimistic
      // value with the authoritative one.
      const key = queryKeys.collections.detail(variables.slug)
      const cached = queryClient.getQueryData<CollectionDetail>(key)
      if (cached) {
        queryClient.setQueryData<CollectionDetail>(key, {
          ...cached,
          like_count: data.like_count,
          user_likes_this: data.user_likes_this,
        })
      }
      // List queries display like_count too — invalidate so they re-fetch
      // (cheap; the user only sees this happen on tab-switch).
      queryClient.invalidateQueries({ queryKey: queryKeys.collections.all })
    },
  })
}

/** Unlike a collection. See useLikeCollection for the optimistic-update story. */
export function useUnlikeCollection() {
  const queryClient = useQueryClient()
  return useMutation<
    CollectionLikeResponse,
    Error,
    { slug: string },
    { previousDetail: CollectionDetail | undefined }
  >({
    mutationFn: ({ slug }) =>
      apiRequest<CollectionLikeResponse>(API_ENDPOINTS.COLLECTIONS.LIKE(slug), {
        method: 'DELETE',
      }),
    onMutate: async ({ slug }) => {
      await queryClient.cancelQueries({
        queryKey: queryKeys.collections.detail(slug),
      })
      const previousDetail = patchDetailLikeCache(queryClient, slug, false)
      return { previousDetail }
    },
    onError: (_err, variables, context) => {
      if (context?.previousDetail) {
        queryClient.setQueryData(
          queryKeys.collections.detail(variables.slug),
          context.previousDetail
        )
      }
    },
    onSuccess: (data, variables) => {
      const key = queryKeys.collections.detail(variables.slug)
      const cached = queryClient.getQueryData<CollectionDetail>(key)
      if (cached) {
        queryClient.setQueryData<CollectionDetail>(key, {
          ...cached,
          like_count: data.like_count,
          user_likes_this: data.user_likes_this,
        })
      }
      queryClient.invalidateQueries({ queryKey: queryKeys.collections.all })
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

// ──────────────────────────────────────────────
// Tags (PSY-354)
// ──────────────────────────────────────────────

/**
 * Apply a tag to a collection. Sends `tag_id` (existing tag) OR `tag_name`
 * (free-form, with alias resolution + inline creation gated by trust tier
 * server-side). Backend enforces the 10-tag cap and edit-access — the
 * mutation surfaces the resulting 4xx via the apiRequest error path.
 *
 * Invalidates the detail query so the chip row refreshes from the server's
 * authoritative list (cheap; the response also includes the new list, but
 * the detail page is the one place where vote counts matter and we'd
 * rather refetch once than try to reconcile two shapes).
 */
export function useAddCollectionTag() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({
      slug,
      tag_id,
      tag_name,
      category,
    }: {
      slug: string
      tag_id?: number
      tag_name?: string
      category?: string
    }) =>
      apiRequest<AddCollectionTagResponse>(API_ENDPOINTS.COLLECTIONS.TAGS(slug), {
        method: 'POST',
        body: JSON.stringify({ tag_id, tag_name, category }),
      }),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.collections.detail(variables.slug),
      })
      // Browse cards display tag chips too — invalidate the public + my
      // lists so the next view reflects the new tag.
      queryClient.invalidateQueries({ queryKey: queryKeys.collections.all })
    },
  })
}

/**
 * Remove a tag from a collection. Server enforces the same edit-access
 * gate as Add. Errors surface via apiRequest.
 */
export function useRemoveCollectionTag() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ slug, tagId }: { slug: string; tagId: number }) =>
      apiRequest<void>(API_ENDPOINTS.COLLECTIONS.TAG(slug, tagId), {
        method: 'DELETE',
      }),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.collections.detail(variables.slug),
      })
      queryClient.invalidateQueries({ queryKey: queryKeys.collections.all })
    },
  })
}

'use client'

/**
 * Tag Hooks
 *
 * TanStack Query hooks for tag browsing, entity tagging, and tag voting.
 */

import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
import type {
  TagDetailResponse,
  TagListResponse,
  TagSearchResponse,
  EntityTagsResponse,
  EntityTag,
} from '../types'

// ──────────────────────────────────────────────
// Queries
// ──────────────────────────────────────────────

interface UseTagsParams {
  category?: string
  search?: string
  parent_id?: number
  sort?: string
  limit?: number
  offset?: number
}

/** Fetch tags list with optional filters */
export function useTags(params?: UseTagsParams) {
  const searchParams = new URLSearchParams()
  if (params?.category) searchParams.set('category', params.category)
  if (params?.search) searchParams.set('search', params.search)
  if (params?.parent_id) searchParams.set('parent_id', String(params.parent_id))
  if (params?.sort) searchParams.set('sort', params.sort)
  if (params?.limit) searchParams.set('limit', String(params.limit))
  if (params?.offset) searchParams.set('offset', String(params.offset))

  const queryString = searchParams.toString()
  const url = queryString
    ? `${API_ENDPOINTS.TAGS.LIST}?${queryString}`
    : API_ENDPOINTS.TAGS.LIST

  return useQuery({
    queryKey: queryKeys.tags.list(params as Record<string, unknown> | undefined),
    queryFn: () => apiRequest<TagListResponse>(url),
    staleTime: 5 * 60 * 1000,
    placeholderData: keepPreviousData,
  })
}

/** Search tags for autocomplete (debounced, enabled when query length >= 2) */
export function useSearchTags(query: string, limit?: number) {
  const searchParams = new URLSearchParams()
  searchParams.set('q', query)
  if (limit) searchParams.set('limit', String(limit))

  const url = `${API_ENDPOINTS.TAGS.SEARCH}?${searchParams.toString()}`

  return useQuery({
    queryKey: queryKeys.tags.search(query),
    queryFn: () => apiRequest<TagSearchResponse>(url),
    enabled: query.length >= 2,
    staleTime: 30 * 1000,
  })
}

/** Fetch a single tag by ID or slug */
export function useTag(idOrSlug: string | number, options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: queryKeys.tags.detail(idOrSlug),
    queryFn: () =>
      apiRequest<TagDetailResponse>(API_ENDPOINTS.TAGS.GET(idOrSlug)),
    enabled: options?.enabled ?? true,
    staleTime: 5 * 60 * 1000,
  })
}

/** Fetch tags on an entity */
export function useEntityTags(
  entityType: string,
  entityId: number,
  options?: { enabled?: boolean }
) {
  return useQuery({
    queryKey: queryKeys.tags.entityTags(entityType, entityId),
    queryFn: () =>
      apiRequest<EntityTagsResponse>(API_ENDPOINTS.ENTITY_TAGS.LIST(entityType, entityId)),
    enabled: (options?.enabled ?? true) && entityId > 0,
    staleTime: 5 * 60 * 1000,
  })
}

// ──────────────────────────────────────────────
// Mutations
// ──────────────────────────────────────────────

/** Add a tag to an entity */
export function useAddTagToEntity() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({
      entityType,
      entityId,
      tag_id,
      tag_name,
    }: {
      entityType: string
      entityId: number
      tag_id?: number
      tag_name?: string
    }) =>
      apiRequest<void>(API_ENDPOINTS.ENTITY_TAGS.ADD(entityType, entityId), {
        method: 'POST',
        body: JSON.stringify({ tag_id, tag_name }),
      }),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.tags.entityTags(variables.entityType, variables.entityId),
      })
      queryClient.invalidateQueries({ queryKey: queryKeys.tags.all })
    },
  })
}

/** Remove a tag from an entity */
export function useRemoveTagFromEntity() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({
      entityType,
      entityId,
      tagId,
    }: {
      entityType: string
      entityId: number
      tagId: number
    }) =>
      apiRequest<void>(API_ENDPOINTS.ENTITY_TAGS.REMOVE(entityType, entityId, tagId), {
        method: 'DELETE',
      }),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.tags.entityTags(variables.entityType, variables.entityId),
      })
      queryClient.invalidateQueries({ queryKey: queryKeys.tags.all })
    },
  })
}

/** Vote on a tag (with optimistic updates) */
export function useVoteOnTag() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({
      tagId,
      entityType,
      entityId,
      is_upvote,
    }: {
      tagId: number
      entityType: string
      entityId: number
      is_upvote: boolean
    }) =>
      apiRequest<void>(API_ENDPOINTS.ENTITY_TAGS.VOTE(tagId, entityType, entityId), {
        method: 'POST',
        body: JSON.stringify({ is_upvote }),
      }),
    onMutate: async ({ tagId, entityType, entityId, is_upvote }) => {
      await queryClient.cancelQueries({
        queryKey: queryKeys.tags.entityTags(entityType, entityId),
      })

      const previousData = queryClient.getQueryData<EntityTagsResponse>(
        queryKeys.tags.entityTags(entityType, entityId)
      )

      if (previousData) {
        const newVote = is_upvote ? 1 : -1
        const updatedTags = previousData.tags.map((tag: EntityTag) => {
          if (tag.tag_id !== tagId) return tag

          const oldVote = tag.user_vote ?? 0
          let upvoteDelta = 0
          let downvoteDelta = 0

          if (oldVote === 1) {
            upvoteDelta = -1
            if (newVote === -1) downvoteDelta = 1
          } else if (oldVote === -1) {
            downvoteDelta = -1
            if (newVote === 1) upvoteDelta = 1
          } else {
            if (newVote === 1) upvoteDelta = 1
            else downvoteDelta = 1
          }

          return {
            ...tag,
            user_vote: newVote,
            upvotes: tag.upvotes + upvoteDelta,
            downvotes: tag.downvotes + downvoteDelta,
          }
        })

        queryClient.setQueryData<EntityTagsResponse>(
          queryKeys.tags.entityTags(entityType, entityId),
          { ...previousData, tags: updatedTags }
        )
      }

      return { previousData }
    },
    onError: (_err, variables, context) => {
      if (context?.previousData) {
        queryClient.setQueryData(
          queryKeys.tags.entityTags(variables.entityType, variables.entityId),
          context.previousData
        )
      }
    },
    onSettled: (_data, _error, variables) => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.tags.entityTags(variables.entityType, variables.entityId),
      })
    },
  })
}

/** Remove a vote from a tag (with optimistic updates) */
export function useRemoveTagVote() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({
      tagId,
      entityType,
      entityId,
    }: {
      tagId: number
      entityType: string
      entityId: number
    }) =>
      apiRequest<void>(API_ENDPOINTS.ENTITY_TAGS.VOTE(tagId, entityType, entityId), {
        method: 'DELETE',
      }),
    onMutate: async ({ tagId, entityType, entityId }) => {
      await queryClient.cancelQueries({
        queryKey: queryKeys.tags.entityTags(entityType, entityId),
      })

      const previousData = queryClient.getQueryData<EntityTagsResponse>(
        queryKeys.tags.entityTags(entityType, entityId)
      )

      if (previousData) {
        const updatedTags = previousData.tags.map((tag: EntityTag) => {
          if (tag.tag_id !== tagId) return tag

          const oldVote = tag.user_vote ?? 0
          let upvoteDelta = 0
          let downvoteDelta = 0

          if (oldVote === 1) upvoteDelta = -1
          else if (oldVote === -1) downvoteDelta = -1

          return {
            ...tag,
            user_vote: null,
            upvotes: tag.upvotes + upvoteDelta,
            downvotes: tag.downvotes + downvoteDelta,
          }
        })

        queryClient.setQueryData<EntityTagsResponse>(
          queryKeys.tags.entityTags(entityType, entityId),
          { ...previousData, tags: updatedTags }
        )
      }

      return { previousData }
    },
    onError: (_err, variables, context) => {
      if (context?.previousData) {
        queryClient.setQueryData(
          queryKeys.tags.entityTags(variables.entityType, variables.entityId),
          context.previousData
        )
      }
    },
    onSettled: (_data, _error, variables) => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.tags.entityTags(variables.entityType, variables.entityId),
      })
    },
  })
}

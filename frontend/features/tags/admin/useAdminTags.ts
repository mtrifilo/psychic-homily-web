'use client'

import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
import type {
  TagDetailResponse,
  TagAliasesResponse,
  TagAliasListingResponse,
  BulkAliasImportItem,
  BulkAliasImportResult,
  MergeTagsPreview,
  MergeTagsResult,
  LowQualityTagQueueResponse,
  GenreHierarchyResponse,
} from '../types'

// ──────────────────────────────────────────────
// Queries
// ──────────────────────────────────────────────

/** Fetch aliases for a tag (admin detail panel) */
export function useTagAliases(tagId: number, options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: queryKeys.tags.aliases(tagId),
    queryFn: () =>
      apiRequest<TagAliasesResponse>(API_ENDPOINTS.TAGS.ALIASES(tagId)),
    enabled: (options?.enabled ?? true) && tagId > 0,
    staleTime: 5 * 60 * 1000,
  })
}

// ──────────────────────────────────────────────
// Mutations
// ──────────────────────────────────────────────

interface CreateTagInput {
  name: string
  description?: string
  parent_id?: number
  category: string
  is_official?: boolean
}

/** Create a new tag (admin only) */
export function useCreateTag() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateTagInput) =>
      apiRequest<TagDetailResponse>(API_ENDPOINTS.TAGS.LIST, {
        method: 'POST',
        body: JSON.stringify(input),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.tags.all })
    },
  })
}

interface UpdateTagInput {
  tagId: number
  data: {
    name?: string
    description?: string | null
    parent_id?: number | null
    category?: string
    is_official?: boolean
  }
}

/** Update an existing tag (admin only) */
export function useUpdateTag() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ tagId, data }: UpdateTagInput) =>
      apiRequest<TagDetailResponse>(API_ENDPOINTS.TAGS.GET(tagId), {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.tags.all })
      queryClient.invalidateQueries({
        queryKey: queryKeys.tags.detail(variables.tagId),
      })
    },
  })
}

/** Delete a tag (admin only) */
export function useDeleteTag() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (tagId: number) =>
      apiRequest<void>(API_ENDPOINTS.TAGS.GET(tagId), {
        method: 'DELETE',
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.tags.all })
    },
  })
}

interface CreateAliasInput {
  tagId: number
  alias: string
}

/** Create a tag alias (admin only) */
export function useCreateAlias() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ tagId, alias }: CreateAliasInput) =>
      apiRequest<void>(API_ENDPOINTS.TAGS.ALIASES(tagId), {
        method: 'POST',
        body: JSON.stringify({ alias }),
      }),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.tags.aliases(variables.tagId),
      })
      queryClient.invalidateQueries({
        queryKey: queryKeys.tags.detail(variables.tagId),
      })
    },
  })
}

interface DeleteAliasInput {
  tagId: number
  aliasId: number
}

/** Delete a tag alias (admin only) */
export function useDeleteAlias() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ tagId, aliasId }: DeleteAliasInput) =>
      apiRequest<void>(API_ENDPOINTS.TAGS.DELETE_ALIAS(tagId, aliasId), {
        method: 'DELETE',
      }),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.tags.aliases(variables.tagId),
      })
      queryClient.invalidateQueries({
        queryKey: queryKeys.tags.detail(variables.tagId),
      })
    },
  })
}

interface ListAllAliasesParams {
  search?: string
  limit?: number
  offset?: number
}

/** Fetch all aliases across all tags (admin global listing) */
export function useAllTagAliases(params: ListAllAliasesParams = {}) {
  const qs = new URLSearchParams()
  if (params.search) qs.set('search', params.search)
  if (params.limit !== undefined) qs.set('limit', String(params.limit))
  if (params.offset !== undefined) qs.set('offset', String(params.offset))
  const url = `${API_ENDPOINTS.TAGS.ADMIN_ALIASES_ALL}${qs.toString() ? `?${qs.toString()}` : ''}`

  const keyParams: Record<string, unknown> = { ...params }

  return useQuery({
    queryKey: queryKeys.tags.allAliases(keyParams),
    queryFn: () => apiRequest<TagAliasListingResponse>(url),
    staleTime: 30 * 1000,
  })
}

/** Bulk import aliases (admin only). Input is already-parsed rows. */
export function useBulkImportAliases() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (items: BulkAliasImportItem[]) =>
      apiRequest<BulkAliasImportResult>(
        API_ENDPOINTS.TAGS.ADMIN_ALIASES_BULK,
        {
          method: 'POST',
          body: JSON.stringify({ items }),
        }
      ),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['tags', 'aliases'] })
      queryClient.invalidateQueries({ queryKey: queryKeys.tags.all })
    },
  })
}

// ──────────────────────────────────────────────
// Merge Tags (PSY-306)
// ──────────────────────────────────────────────

/**
 * Preview what a merge would do without committing. Used by the merge dialog
 * to show counts before the admin confirms. Disabled until both IDs are set
 * and the user opts in (enabled flag), so we don't spam the backend while
 * the dialog is still in target-selection mode.
 */
export function useMergeTagsPreview(
  sourceId: number | null,
  targetId: number | null,
  options?: { enabled?: boolean }
) {
  return useQuery({
    queryKey: ['tags', 'merge-preview', sourceId, targetId],
    queryFn: () =>
      apiRequest<MergeTagsPreview>(
        API_ENDPOINTS.TAGS.ADMIN_MERGE_PREVIEW(sourceId as number, targetId as number)
      ),
    enabled:
      (options?.enabled ?? true) &&
      sourceId !== null &&
      targetId !== null &&
      sourceId > 0 &&
      targetId > 0 &&
      sourceId !== targetId,
    staleTime: 0,
  })
}

interface MergeTagsInput {
  sourceId: number
  targetId: number
}

/** Merge source tag into target tag (admin only). Invalidates tag lists + aliases. */
export function useMergeTags() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ sourceId, targetId }: MergeTagsInput) =>
      apiRequest<MergeTagsResult>(API_ENDPOINTS.TAGS.ADMIN_MERGE(sourceId), {
        method: 'POST',
        body: JSON.stringify({ target_id: targetId }),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.tags.all })
      queryClient.invalidateQueries({ queryKey: ['tags', 'aliases'] })
    },
  })
}

// ──────────────────────────────────────────────
// Low-Quality Tag Queue (PSY-310)
// ──────────────────────────────────────────────

interface UseLowQualityTagQueueParams {
  limit?: number
  offset?: number
}

/**
 * Fetch the admin low-quality tag review queue. Short staleTime (30s) so
 * Ignore/Promote/Delete actions reflect quickly without polling.
 */
export function useLowQualityTagQueue(params: UseLowQualityTagQueueParams = {}) {
  const qs = new URLSearchParams()
  if (params.limit !== undefined) qs.set('limit', String(params.limit))
  if (params.offset !== undefined) qs.set('offset', String(params.offset))
  const url = `${API_ENDPOINTS.TAGS.ADMIN_LOW_QUALITY}${qs.toString() ? `?${qs.toString()}` : ''}`

  return useQuery({
    queryKey: queryKeys.tags.lowQuality(params as Record<string, unknown>),
    queryFn: () => apiRequest<LowQualityTagQueueResponse>(url),
    staleTime: 30 * 1000,
    placeholderData: keepPreviousData,
  })
}

/** Snooze a tag (mark reviewed). Invalidates the queue + tag lists. */
export function useSnoozeTag() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (tagId: number) =>
      apiRequest<void>(API_ENDPOINTS.TAGS.ADMIN_SNOOZE(tagId), {
        method: 'POST',
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.tags.lowQuality() })
      queryClient.invalidateQueries({ queryKey: queryKeys.tags.all })
    },
  })
}

/**
 * Promote a tag to official by reusing the existing update endpoint
 * (PUT /tags/{id}) with `is_official: true`. No new backend endpoint needed.
 */
export function useMarkTagOfficial() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (tagId: number) =>
      apiRequest<TagDetailResponse>(API_ENDPOINTS.TAGS.GET(tagId), {
        method: 'PUT',
        body: JSON.stringify({ is_official: true }),
      }),
    onSuccess: (_data, tagId) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.tags.lowQuality() })
      queryClient.invalidateQueries({ queryKey: queryKeys.tags.all })
      queryClient.invalidateQueries({ queryKey: queryKeys.tags.detail(tagId) })
    },
  })
}

// ──────────────────────────────────────────────
// Genre hierarchy (PSY-311)
// ──────────────────────────────────────────────

/**
 * Fetch all genre tags as a flat list with parent_id (admin only).
 * The frontend builds the tree client-side. 30-second staleTime keeps
 * the editor snappy after mutations without thrashing the backend.
 */
export function useGenreHierarchy(options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: queryKeys.tags.genreHierarchy,
    queryFn: () =>
      apiRequest<GenreHierarchyResponse>(API_ENDPOINTS.TAGS.ADMIN_HIERARCHY),
    enabled: options?.enabled ?? true,
    staleTime: 30 * 1000,
  })
}

interface SetTagParentInput {
  tagId: number
  /** Pass null (not undefined) to clear the parent. */
  parentId: number | null
}

/**
 * Set or clear the parent of a genre tag (admin only). Cycle detection and
 * category enforcement live on the backend; this mutation just surfaces the
 * error message. Invalidates the genre hierarchy + the affected tag's detail
 * so the detail page parent/children section stays fresh.
 */
export function useSetTagParent() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ tagId, parentId }: SetTagParentInput) =>
      apiRequest<void>(API_ENDPOINTS.TAGS.ADMIN_SET_PARENT(tagId), {
        method: 'PATCH',
        body: JSON.stringify({ parent_id: parentId }),
      }),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.tags.genreHierarchy })
      queryClient.invalidateQueries({ queryKey: queryKeys.tags.all })
      queryClient.invalidateQueries({
        queryKey: queryKeys.tags.detail(variables.tagId),
      })
      queryClient.invalidateQueries({
        queryKey: queryKeys.tags.enrichedDetail(variables.tagId),
      })
    },
  })
}

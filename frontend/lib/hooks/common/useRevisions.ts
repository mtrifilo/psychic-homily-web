import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'

// --- Types ---

export interface FieldChange {
  field: string
  old_value: unknown
  new_value: unknown
}

export interface RevisionItem {
  id: number
  entity_type: string
  entity_id: number
  user_id: number
  user_name?: string
  changes: FieldChange[]
  summary?: string
  created_at: string
}

interface EntityHistoryResponse {
  revisions: RevisionItem[]
  total: number
}

interface UserRevisionsResponse {
  revisions: RevisionItem[]
  total: number
}

interface RollbackResponse {
  success: boolean
}

// --- Hooks ---

/**
 * Fetch revision history for a specific entity.
 */
export function useEntityRevisions(
  entityType: string,
  entityId: string | number,
  options?: { enabled?: boolean; limit?: number; offset?: number }
) {
  const limit = options?.limit ?? 20
  const offset = options?.offset ?? 0

  return useQuery({
    queryKey: [...queryKeys.revisions.entity(entityType, entityId), { limit, offset }],
    queryFn: () => {
      const url = new URL(API_ENDPOINTS.REVISIONS.ENTITY_HISTORY(entityType, entityId))
      if (limit) url.searchParams.set('limit', String(limit))
      if (offset) url.searchParams.set('offset', String(offset))
      return apiRequest<EntityHistoryResponse>(url.toString())
    },
    enabled: options?.enabled !== false,
  })
}

/**
 * Fetch a single revision by ID.
 */
export function useRevision(
  revisionId: number,
  options?: { enabled?: boolean }
) {
  return useQuery({
    queryKey: queryKeys.revisions.detail(revisionId),
    queryFn: () =>
      apiRequest<RevisionItem>(API_ENDPOINTS.REVISIONS.DETAIL(revisionId)),
    enabled: options?.enabled !== false && revisionId > 0,
  })
}

/**
 * Fetch revision history for a specific user.
 */
export function useUserRevisions(
  userId: string | number,
  options?: { enabled?: boolean; limit?: number; offset?: number }
) {
  const limit = options?.limit ?? 20
  const offset = options?.offset ?? 0

  return useQuery({
    queryKey: [...queryKeys.revisions.user(userId), { limit, offset }],
    queryFn: () => {
      const url = new URL(API_ENDPOINTS.REVISIONS.USER_REVISIONS(userId))
      if (limit) url.searchParams.set('limit', String(limit))
      if (offset) url.searchParams.set('offset', String(offset))
      return apiRequest<UserRevisionsResponse>(url.toString())
    },
    enabled: options?.enabled !== false,
  })
}

/**
 * Rollback a revision (admin only).
 */
export function useRollbackRevision() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (revisionId: number) =>
      apiRequest<RollbackResponse>(API_ENDPOINTS.REVISIONS.ROLLBACK(revisionId), {
        method: 'POST',
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.revisions.all })
    },
  })
}
